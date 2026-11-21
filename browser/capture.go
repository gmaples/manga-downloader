// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Snapshot is what Capture saw of a page
type Snapshot struct {
	// HTML is the page once its scripts ran and it was scrolled to the bottom
	HTML string
	// Status is the HTTP status of the page itself, 0 when none arrived
	Status int
	// JSON are the JSON bodies the page's own scripts fetched while it loaded
	JSON []APIResponse
}

// Limits of what one Capture keeps, so a page that streams data can't fill
// the memory: the number of JSON bodies, the size of each, and the scroll
// steps taken (an endless feed never reaches its bottom).
const (
	captureMaxJSON  = 200
	captureMaxBody  = 4 << 20
	captureMaxSteps = 60
)

// captureLoadWait is how long Capture lets a page's scripts run after it
// loaded, before scrolling. A var so tests can shrink it.
var captureLoadWait = 2 * time.Second

// Capture renders pageURL for a reader that knows nothing about the site: it
// waits for the page's scripts, scrolls it a viewport at a time down to the
// bottom (at most captureMaxSteps steps) so lazy lists and images mount, and
// returns the HTML along with the JSON the page fetched meanwhile.
//
// prepare, if set, is JS run once the page's scripts had their time and
// before the scroll, e.g. to click a tab that reveals a list. When it returns
// true the page gets captureLoadWait again to fetch and draw what it revealed.
//
// It never opens a window by itself: a headless browser is used unless the
// user asked for a visible one, and a page behind a challenge isn't retried
// in a window, as GetHTML would. Callers check NeedsVisible first for the
// hosts known to refuse a headless browser.
func Capture(pageURL, prepare string, timeout time.Duration) (Snapshot, error) {
	abort := lock()
	defer mu.Unlock()

	var snap Snapshot
	err := runPage(abort, pageURL, timeout, false, func(ctx context.Context, h hooks) error {
		var err error
		snap, err = capture(ctx, h, pageURL, prepare)
		return err
	})
	return snap, err
}

// capture is one Capture attempt in the shared browser's tab
func capture(ctx context.Context, h hooks, pageURL, prepare string) (Snapshot, error) {
	type entry struct {
		id  network.RequestID
		url string
	}
	var (
		cmu     sync.Mutex
		entries []entry
		status  int
	)
	var main cdp.FrameID
	if c := chromedp.FromContext(ctx); c != nil && c.Target != nil {
		main = cdp.FrameID(c.Target.TargetID)
	}
	listen(chromedp.Events(ctx, network.ResponseReceived), func(resp network.EventResponseReceived) {
		if NetLog != nil {
			NetLog(resp.Response.URL, int(resp.Response.Status), resp.Response.MimeType)
		}
		cmu.Lock()
		defer cmu.Unlock()
		if resp.Type == network.ResourceTypeDocument && resp.FrameID == main {
			// the last document wins: a redirect or a challenge handover
			// replaces the first one
			status = int(resp.Response.Status)
			return
		}
		if strings.Contains(strings.ToLower(resp.Response.MimeType), "json") && len(entries) < captureMaxJSON {
			entries = append(entries, entry{resp.RequestID, resp.Response.URL})
		}
	})

	if err := chromedp.Do(ctx, send(network.Enable, network.EnableParams{}), chromedp.Navigate(pageURL)); err != nil {
		return Snapshot{}, err
	}
	h.rendered()
	// scrolling and reading the bodies get their own budget
	h.rearm(afterRenderTimeout)

	snap := Snapshot{}
	err := chromedp.Do(ctx,
		chromedp.Sleep(captureLoadWait),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			if prepare == "" {
				return nil
			}
			acted, err := chromedp.Evaluate[bool](prepare)(ctx, t)
			if err != nil || !acted {
				// a page the script can't act on is read as it is
				return nil
			}
			_, err = chromedp.Sleep(captureLoadWait)(ctx, t)
			return err
		}),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			js := scrollStepJS(Scroll{})
			atBottom := 0
			for range captureMaxSteps {
				raw, err := chromedp.Evaluate[string](js)(ctx, t)
				if err != nil {
					return err
				}
				step, err := parseScrollStep(raw)
				if err != nil {
					return err
				}
				if step.State == "bottom" {
					atBottom++
					if atBottom > scrollSettleSteps {
						return nil
					}
				} else {
					atBottom = 0
				}
				if _, err := chromedp.Sleep(400*time.Millisecond)(ctx, t); err != nil {
					return err
				}
			}
			return nil
		}),
		chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			var err error
			snap.HTML, err = documentHTML(ctx, t)
			return err
		}),
		chromedp.Func(harvestSession),
	)
	if err != nil {
		return Snapshot{}, err
	}

	cmu.Lock()
	snap.Status = status
	list := append([]entry(nil), entries...)
	cmu.Unlock()
	for _, e := range list {
		id := e.id
		var body string
		_ = chromedp.Do(ctx, chromedp.Func(func(ctx context.Context, t *chromedp.Target) error {
			res, err := cdp.Call(ctx, t, network.GetResponseBody, network.GetResponseBodyParams{RequestID: id})
			if err == nil && !res.Base64encoded && len(res.Body) <= captureMaxBody {
				body = string(res.Body)
			}
			return nil
		}))
		if body != "" {
			snap.JSON = append(snap.JSON, APIResponse{URL: e.url, Body: body})
		}
	}
	return snap, nil
}
