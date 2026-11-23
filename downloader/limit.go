// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"context"
	"errors"
	"io"
	"net"
	stdhttp "net/http"
	"net/url"
	"time"

	"github.com/elboletaire/manga-downloader/http"
)

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

// tooMany reports whether err is a 429 Too Many Requests
func tooMany(err error) bool {
	var se *http.StatusError
	return errors.As(err, &se) && se.Code == stdhttp.StatusTooManyRequests
}

// TooManyRetry reports whether err is a 429 that lim says to wait out and
// retry (lim.Hit didn't give up). lim may be nil.
func TooManyRetry(lim Limiter, gen uint64, err error) bool {
	var se *http.StatusError
	if lim == nil || !errors.As(err, &se) || se.Code != stdhttp.StatusTooManyRequests {
		return false
	}
	return !lim.Hit(gen, se.RetryAfter)
}
