// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	stdhttp "net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/notice"
)

// retryDelay is the base delay between retry attempts (multiplied by the
// attempt number). rateLimitDelay is the wait after a 429 whose response
// didn't say how long to back off: a rate limit needs longer than a blip.
// Package-level vars so tests can shrink them.
var (
	retryDelay     = time.Second
	rateLimitDelay = 10 * time.Second
)

// File represents a downloaded file
type File struct {
	Data []byte
	Page uint
}

// ProgressCallback is a function type for progress updates with optional error
type ProgressCallback func(page, progress int, err error)

// Limiter paces a site's requests and slows them down on a 429
// (ratelimit.Site). Wait blocks until a request may go out and returns its
// generation; Hit records a 429 to a request of that generation and reports
// whether to give up. Listed reports whether it has a pace of its own, which
// replaces the site's PagePacer. Done ends a request Wait let out, with the
// bytes it downloaded (negative for a request that isn't a page) and whether
// it failed with Trouble, which is how the site's level of requests under way
// at once is learned.
type Limiter interface {
	Wait(ctx context.Context) (uint64, error)
	Hit(gen uint64, retryAfter time.Duration) (giveUp bool)
	Listed() bool
	Done(bytes int64, trouble bool)
}

// Trouble reports whether err says the site is overloaded rather than that
// the request was wrong: a 429, a 502, 503 or 504, a Cloudflare 520-524, a
// timeout or a connection cut short. A 404 or a 500 isn't.
func Trouble(err error) bool {
	if err == nil {
		return false
	}
	var se *http.StatusError
	if errors.As(err, &se) {
		switch c := se.Code; {
		case c == stdhttp.StatusTooManyRequests, c == stdhttp.StatusBadGateway, c == stdhttp.StatusServiceUnavailable, c == stdhttp.StatusGatewayTimeout:
			return true
		case c >= 520 && c <= 524:
			return true
		}
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	// a reset reads as a failed read on every system (ECONNRESET, WSAECONNRESET)
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Op == "read" {
		return true
	}
	return errors.Is(err, io.ErrUnexpectedEOF)
}

// Mirrorer is a Limiter that also knows other servers for a page's host
// (ratelimit.Site, from site-mirrors.txt)
type Mirrorer interface {
	Mirrors(host string) []string
}

// mirrorURLs returns rawURL on each other host lim names for its host, in
// order, nil when lim names none
func mirrorURLs(lim Limiter, rawURL string) []string {
	m, ok := lim.(Mirrorer)
	if !ok {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return nil
	}
	var out []string
	for _, h := range m.Mirrors(u.Host) {
		v := *u
		v.Host = h
		out = append(out, v.String())
	}
	return out
}

// hostOf is rawURL's host, rawURL itself when it has none
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return rawURL
}

// FetchChapter downloads all the pages of a chapter. Cancelling ctx cuts
// short the waits between retries; requests already under way still finish.
func FetchChapter(ctx context.Context, site grabber.Site, chapter *grabber.Chapter, onprogress ProgressCallback) ([]*File, error) {
	return FetchChapterLimited(ctx, site, chapter, nil, onprogress)
}

// FetchChapterLimited is FetchChapter with lim pacing every page request,
// in place of the site's own PagePacer once lim has a pace (Limiter). A page
// that gets a 429 waits as lim says and is fetched again, as often as lim
// allows, without using up its retries; the other pages are kept.
func FetchChapterLimited(ctx context.Context, site grabber.Site, chapter *grabber.Chapter, lim Limiter, onprogress ProgressCallback) ([]*File, error) {
	var pace func()
	if p, ok := site.(grabber.PagePacer); ok {
		pace = p.WaitPage
	}

	wg := sync.WaitGroup{}
	guard := make(chan struct{}, site.GetMaxConcurrency().Pages)
	errChan := make(chan error, 1)
	done := make(chan bool)
	// A local (not the named return) so page goroutines write into a slice that
	// can never be nilled out from under them. If a page fails and the function
	// returns early below, `return nil, err` sets the named return to nil - any
	// page goroutine still downloading would then write to a nil slice and panic
	// ("index out of range"). res is only handed to the caller on the success
	// path, so such late writes are just garbage the GC reclaims.
	res := make([]*File, len(chapter.Pages)) // Pre-allocate slice with correct size

	for i, page := range chapter.Pages {
		guard <- struct{}{}
		wg.Add(1)
		go func(page grabber.Page, idx int) {
			defer wg.Done()

			referer := site.BaseUrl()
			if page.NoReferer {
				referer = ""
			}
			params := http.RequestParams{
				URL:     page.URL,
				Referer: referer,
				Headers: page.Headers,
				Accept:  http.ImageAccept,
			}
			file, err := fetchFile(ctx, params, uint(page.Number), site.GetRetries(), page.Transform, pace, page.RetryFlaky, lim)
			if err != nil && ctx.Err() == nil {
				// the other servers site-mirrors.txt names for the page's host
				for _, u := range mirrorURLs(lim, page.URL) {
					params.URL = u
					f, merr := fetchFile(ctx, params, uint(page.Number), site.GetRetries(), page.Transform, pace, page.RetryFlaky, lim)
					if merr == nil {
						notice.Info("page %d: %s failed (%s), got it from %s", page.Number, hostOf(page.URL), err, hostOf(u))
						file, err = f, nil
						break
					}
				}
			}

			if err != nil {
				select {
				case errChan <- fmt.Errorf("page %d: %w", page.Number, err):
					onprogress(idx, idx, err)
				default:
				}
				<-guard
				return
			}

			res[idx] = file         // Store file directly in pre-allocated slice
			onprogress(1, idx, nil) // Progress by 1 page at a time
			<-guard
		}(page, i)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errChan:
		close(guard)
		return nil, err
	case <-done:
		close(guard)
	}

	// sort files by page number
	sort.SliceStable(res, func(i, j int) bool {
		return res[i].Page < res[j].Page
	})

	return res, nil
}

// FetchFile gets an online file returning a new *File with its contents.
// On failure (either the GET itself, a mid-body read, or the optional
// transform) it retries up to `retries` additional times, with a short
// growing delay between attempts (or the server's Retry-After after a 429,
// see retryWait). An HTML body fails at once, without retries. transform, if
// non-nil, post-processes the downloaded bytes.
// Cancelling ctx abandons the wait before a retry.
func FetchFile(ctx context.Context, params http.RequestParams, page uint, retries uint8, transform func([]byte) ([]byte, error)) (*File, error) {
	return fetchFile(ctx, params, page, retries, transform, nil, false, nil)
}

// fetchFile is FetchFile with an optional pace, called before every attempt,
// retryFlaky (Page.RetryFlaky) and lim, which replaces pace once it has a pace
// of its own and handles 429s: they're retried as long as lim allows, not
// counting as attempts.
func fetchFile(ctx context.Context, params http.RequestParams, page uint, retries uint8, transform func([]byte) ([]byte, error), pace func(), retryFlaky bool, lim Limiter) (file *File, err error) {
	orig := params.URL
	for attempt := uint8(0); ; attempt++ {
		var gen uint64
		if lim != nil {
			if gen, err = lim.Wait(ctx); err != nil {
				return nil, err
			}
		}
		if pace != nil && (lim == nil || !lim.Listed()) {
			pace()
		}
		var data []byte
		data, err = fetchFileOnce(params)
		if lim != nil {
			lim.Done(int64(len(data)), Trouble(err))
		}
		if err == nil {
			err = checkPage(data)
		}
		if err == nil && transform != nil {
			data, err = transform(data)
		}
		if err == nil {
			err = checkPage(data)
		}
		if err == nil {
			file = &File{
				Data: data,
				Page: page,
			}
			return
		}
		if errors.Is(err, errHTMLPage) {
			return nil, err
		}
		var se *http.StatusError
		if lim != nil && errors.As(err, &se) && se.Code == stdhttp.StatusTooManyRequests {
			if lim.Hit(gen, se.RetryAfter) {
				return nil, err
			}
			// lim.Wait sits out the cooldown; a 429 isn't one of the retries
			attempt--
			continue
		}

		wait := retryWait(err, attempt)
		limit := retries
		if retryFlaky && retries > 0 && isFlaky(err) {
			limit, wait = max(retries, flakyRetries), retryDelay
		}
		if attempt >= limit {
			return
		}
		if retryFlaky && isFlaky(err) {
			params.URL = bustCache(orig)
		}

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, fmt.Errorf("%w (while waiting to retry after: %w)", ctx.Err(), err)
		case <-t.C:
		}
	}
}

// flakyRetries is how many times an opted-in page that fails is retried at least.
const flakyRetries = 6

// isFlaky reports whether err is a 404 or a 5xx, the failures an opted-in
// origin gives at random (a 429 keeps its own Retry-After handling).
func isFlaky(err error) bool {
	var se *http.StatusError
	return errors.As(err, &se) && (se.Code == stdhttp.StatusNotFound || se.Code >= 500)
}

// bustCache appends a unique query to the raw URL, leaving any existing one
// byte for byte (it may be signed), so a retry skips a CDN's cached 404.
func bustCache(rawURL string) string {
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "retry=" + strconv.FormatInt(time.Now().UnixNano(), 36)
}

// retryWait is the delay before retrying after attempt (0-based) failed with
// err. A 429 waits out its Retry-After (or rateLimitDelay): the linear delay
// would retry while the rate limit still applies.
func retryWait(err error, attempt uint8) time.Duration {
	var se *http.StatusError
	if errors.As(err, &se) && se.Code == stdhttp.StatusTooManyRequests {
		if se.RetryAfter > 0 {
			return se.RetryAfter
		}
		return rateLimitDelay
	}
	return retryDelay * time.Duration(attempt+1)
}

// fetchFileOnce performs a single GET + body read attempt. DisableRetry is
// set so the HTTP package's own retry loop doesn't fire — FetchFile has its
// own retry loop and the two would otherwise multiply for every 5xx page.
func fetchFileOnce(params http.RequestParams) (data []byte, err error) {
	params.DisableRetry = true
	body, err := http.Get(params)
	if err != nil {
		return
	}
	defer body.Close()

	data, err = io.ReadAll(body)
	return
}

// errHTMLPage is what a page answered with an HTML document (a challenge or
// error page, usually with a 200) gets. Refetching it gives the same page,
// so fetchFile doesn't retry it.
var errHTMLPage = errors.New("not an image (got an HTML page)")

// checkPage returns errHTMLPage when data is an HTML document. Any other
// bytes pass, including formats imgfmt doesn't know, and SVG (XML).
func checkPage(data []byte) error {
	if isHTMLPage(data) {
		return errHTMLPage
	}
	return nil
}

// isHTMLPage matches only HTML openers. http.DetectContentType isn't used:
// it calls an SVG that starts with a comment text/html.
func isHTMLPage(data []byte) bool {
	head := bytes.TrimLeft(data, "\t\r\n \ufeff")
	head = head[:min(len(head), 16)]
	lower := bytes.ToLower(head)
	for _, opener := range []string{"<!doctype html", "<html", "<head", "<body"} {
		if bytes.HasPrefix(lower, []byte(opener)) {
			return true
		}
	}
	return false
}
