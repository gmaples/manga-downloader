// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	mdhttp "github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// learnFast shrinks the learning values (and the 429 pacing) for a test
func learnFast(t *testing.T) {
	t.Helper()
	w, h, p := ratelimit.Window, ratelimit.Hold, ratelimit.ProbeEvery
	ratelimit.Window, ratelimit.Hold, ratelimit.ProbeEvery = 40*time.Millisecond, 150*time.Millisecond, 2
	t.Cleanup(func() { ratelimit.Window, ratelimit.Hold, ratelimit.ProbeEvery = w, h, p })
	fastLimits(t, 2*time.Second)
	g, mg, sw := ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait
	ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait = 0, 0, 2*time.Millisecond
	t.Cleanup(func() { ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait = g, mg, sw })
}

// mockSite is a server that serves pages with `workers` at once, each taking
// `serve`; past that, requests queue (slow) or answer 429 (touchy). It records
// the most requests it saw under way at once.
type mockSite struct {
	server   *httptest.Server
	workers  chan struct{}
	touchy   bool
	inflight atomic.Int32
	most     atomic.Int32
	limited  atomic.Int32
	served   atomic.Int32
}

func newMockSite(t *testing.T, workers int, serve time.Duration, touchy bool) *mockSite {
	t.Helper()
	m := &mockSite{workers: make(chan struct{}, workers), touchy: touchy}
	page := make([]byte, 4096)
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := m.inflight.Add(1)
		defer m.inflight.Add(-1)
		for {
			most := m.most.Load()
			if n <= most || m.most.CompareAndSwap(most, n) {
				break
			}
		}
		if m.touchy {
			select {
			case m.workers <- struct{}{}:
			default:
				m.limited.Add(1)
				http.Error(w, "slow down", http.StatusTooManyRequests)
				return
			}
		} else {
			m.workers <- struct{}{}
		}
		time.Sleep(serve)
		<-m.workers
		m.served.Add(1)
		_, _ = w.Write(page)
	}))
	t.Cleanup(m.server.Close)
	return m
}

// download fetches `chapters` chapters of 10 pages from m, 5 at a time as the
// app does, through the shared pace of root, and returns the levels the site
// went through and what the pace reported
func (m *mockSite) download(t *testing.T, root string, chapters int) (levels []int, said []string) {
	t.Helper()
	site := fetchChapterTestSite{Grabber: &grabber.Grabber{URL: m.server.URL, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 5, Pages: 10},
		Retry:          2,
	}}}
	var saidMu sync.Mutex
	info := func(format string, a ...any) {
		saidMu.Lock()
		defer saidMu.Unlock()
		said = append(said, fmt.Sprintf(format, a...))
	}
	book, release := ratelimit.Share(root, nil, info)
	defer release()
	lim := book.Site("example.com")

	stop := make(chan struct{})
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		last := 0
		for {
			if n := lim.Parallel(); n != last {
				levels = append(levels, n)
				last = n
			}
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	g := make(chan struct{}, 5)
	var wg sync.WaitGroup
	var failed atomic.Int32
	for c := range chapters {
		g <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-g }()
			chapter := &grabber.Chapter{Number: float64(c + 1)}
			for p := range 10 {
				chapter.Pages = append(chapter.Pages, grabber.Page{Number: int64(p + 1), URL: fmt.Sprintf("%s/%d/%d", m.server.URL, c, p)})
			}
			if _, err := FetchChapterLimited(context.Background(), site, chapter, lim, func(int, int, error) {}); err != nil {
				failed.Add(1)
				t.Errorf("chapter %d: %v", c+1, err)
			}
		}()
	}
	wg.Wait()
	close(stop)
	<-watched
	if failed.Load() > 0 {
		t.Fatalf("%d chapters failed", failed.Load())
	}
	return levels, said
}

// A slow site that serves 8 pages at once gains nothing past 8: the level
// doubles to 16, finds it no faster, settles back at 8 (tries of a few more
// may be kept or under way at the end), and that's saved for the next run, which starts there
func TestLearnsASlowSite(t *testing.T) {
	learnFast(t)
	m := newMockSite(t, 8, 4*time.Millisecond, false)
	root := t.TempDir()
	levels, said := m.download(t, root, 150)
	t.Logf("levels: %v, most at once on the server: %d, reported: %q", levels, m.most.Load(), said)

	// a loaded test machine can drop a connection, which rightly cuts the
	// level; the path is checked only for a run without one
	cut := slices.ContainsFunc(said, func(s string) bool { return strings.Contains(s, "struggling") })
	if !cut && (len(levels) < 3 || levels[0] != 4 || levels[1] != 8 || levels[2] != 16) {
		t.Errorf("levels = %v, want 4, 8, 16 first", levels)
	}
	final := levels[len(levels)-1]
	// a try of one more is kept when it measures 5% faster, which timing
	// noise can give now and then, so the level may sit a few above 8
	if final < 8 || final > 12 {
		t.Errorf("final level = %d, want 8 or a few more", final)
	}
	for _, n := range levels[min(3, len(levels)):] {
		if n > 12 {
			t.Errorf("went back up to %d after settling: %v", n, levels)
			break
		}
	}
	data, err := os.ReadFile(filepath.Join(root, ratelimit.ParallelFileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("\nexample.com: %d\n", final); !strings.Contains(string(data), want) {
		t.Errorf("file =\n%s\nwant %q", data, want)
	}
	if _, err := os.Stat(filepath.Join(root, ratelimit.FileName)); !os.IsNotExist(err) {
		t.Errorf("a site that never answered 429 got a rate limit: %v", err)
	}

	// the next run starts from the saved level, not from 4
	book, release := ratelimit.Share(root, nil, nil)
	defer release()
	if n := book.Site("example.com").Parallel(); n != final {
		t.Errorf("the next run starts at %d, want the saved %d", n, final)
	}
}

// A touchy site that answers 429 past 6 at once: every page still comes out,
// the level is cut below 6 at the first 429s and never climbs back to the
// level that had them, so the 429s stay few
func TestLearnsATouchySite(t *testing.T) {
	learnFast(t)
	m := newMockSite(t, 6, 4*time.Millisecond, true)
	root := t.TempDir()
	levels, said := m.download(t, root, 100)
	t.Logf("levels: %v, 429s: %d of %d served, reported: %q", levels, m.limited.Load(), m.served.Load(), said)

	if got := m.served.Load(); got != 1000 {
		t.Errorf("served %d pages, want 1000", got)
	}
	final := levels[len(levels)-1]
	if final > 7 {
		t.Errorf("final level = %d, want below 8, the level that had 429s", final)
	}
	if final < 2 {
		t.Errorf("final level = %d, cut further than needed", final)
	}
	// 4 then 8 has the first 429s; after the cut the level may rise to 7 at
	// most, which can have them once more
	if n := m.limited.Load(); n > 100 {
		t.Errorf("%d 429s, want few", n)
	}
	data, err := os.ReadFile(filepath.Join(root, ratelimit.ParallelFileName))
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("\nexample.com: %d\n", final); !strings.Contains(string(data), want) {
		t.Errorf("file =\n%s\nwant %q", data, want)
	}
}

func TestTrouble(t *testing.T) {
	timeout := &net.OpError{Op: "dial", Err: timeoutErr{}}
	reset := &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}
	for _, c := range []struct {
		err  error
		want bool
	}{
		{nil, false},
		{&mdhttp.StatusError{Code: 429}, true},
		{fmt.Errorf("page 2: %w", &mdhttp.StatusError{Code: 503}), true},
		{&mdhttp.StatusError{Code: 502}, true},
		{&mdhttp.StatusError{Code: 504}, true},
		{&mdhttp.StatusError{Code: 522}, true},
		{&mdhttp.StatusError{Code: 500}, false},
		{&mdhttp.StatusError{Code: 404}, false},
		{&mdhttp.StatusError{Code: 403}, false},
		{fmt.Errorf("timed out: %w", timeout), true},
		{reset, true},
		{io.ErrUnexpectedEOF, true},
		{errHTMLPage, false},
		{errors.New("bad image"), false},
	} {
		if got := Trouble(c.err); got != c.want {
			t.Errorf("Trouble(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
