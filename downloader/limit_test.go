// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// fastLimits shrinks the ratelimit values for a test
func fastLimits(t *testing.T, maxWait time.Duration) {
	t.Helper()
	g, mg, w, mw := ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait
	ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait = 5*time.Millisecond, time.Second, 10*time.Millisecond, maxWait
	t.Cleanup(func() { ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait = g, mg, w, mw })
}

// limitedPages serves 3 pages, page 2 answering 429 for its first `limited`
// requests; it records each request's path
func limitedPages(t *testing.T, limited int) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var seen []string
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		if r.URL.Path == "/2" {
			count++
			if count <= limited {
				mu.Unlock()
				w.Header().Set("Retry-After", "60")
				http.Error(w, "slow down", http.StatusTooManyRequests)
				return
			}
		}
		mu.Unlock()
		_, _ = w.Write([]byte("page" + r.URL.Path))
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seen...)
	}
}

func limitedChapter(url string) *grabber.Chapter {
	return &grabber.Chapter{Number: 1, Pages: []grabber.Page{
		{Number: 1, URL: url + "/1"},
		{Number: 2, URL: url + "/2"},
		{Number: 3, URL: url + "/3"},
	}}
}

// A page that gets 429s is fetched again after the cooldown, without using
// its retries (there are none), the other pages aren't fetched again, the
// chapter comes out whole, and the site's gap is saved in the root
func TestFetchChapterLimited_RetriesA429Page(t *testing.T) {
	fastLimits(t, time.Second)
	server, seen := limitedPages(t, 2)
	site := fetchChapterTestSite{Grabber: &grabber.Grabber{URL: server.URL, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 3},
		Retry:          0,
	}}}
	root := t.TempDir()
	lim := ratelimit.Open(root, nil, nil).Site("example.com")

	files, err := FetchChapterLimited(context.Background(), site, limitedChapter(server.URL), lim, func(int, int, error) {})
	if err != nil {
		t.Fatalf("FetchChapterLimited: %v", err)
	}
	for i, f := range files {
		if want := []byte("page/" + string(rune('1'+i))); !bytes.Equal(f.Data, want) {
			t.Errorf("page %d = %q, want %q", i+1, f.Data, want)
		}
	}
	counts := map[string]int{}
	for _, p := range seen() {
		counts[p]++
	}
	if counts["/1"] != 1 || counts["/3"] != 1 || counts["/2"] != 3 {
		t.Errorf("requests per page = %v, want /2 three times and the others once", counts)
	}
	data, err := os.ReadFile(filepath.Join(root, ratelimit.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "example.com: 8ms\n") {
		t.Errorf("file =\n%s\nwant example.com at 5ms x 1.5, rounded to 8ms", data)
	}
}

// A page still getting 429s once the cooldown is at its cap fails the chapter
func TestFetchChapterLimited_GivesUp(t *testing.T) {
	fastLimits(t, 20*time.Millisecond)
	server, _ := limitedPages(t, 1000)
	site := fetchChapterTestSite{Grabber: &grabber.Grabber{URL: server.URL, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 1},
		Retry:          3,
	}}}
	lim := ratelimit.Open(t.TempDir(), nil, nil).Site("example.com")
	start := time.Now()
	_, err := FetchChapterLimited(context.Background(), site, limitedChapter(server.URL), lim, func(int, int, error) {})
	if err == nil || !strings.Contains(err.Error(), "page 2") || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want page 2's 429", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("gave up after %v", elapsed)
	}
}

// Once the limiter has a pace, the site's own PagePacer is no longer asked:
// the file wins
func TestFetchChapterLimited_FileWinsOverPagePacer(t *testing.T) {
	server, _ := limitedPages(t, 0)
	site := &pacedSite{fetchChapterTestSite: fetchChapterTestSite{Grabber: &grabber.Grabber{URL: server.URL, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 1},
	}}}}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, ratelimit.FileName), []byte("example.com: 1ms\n"), 0o644)
	if _, err := FetchChapterLimited(context.Background(), site, limitedChapter(server.URL), ratelimit.Open(root, nil, nil).Site("example.com"), func(int, int, error) {}); err != nil {
		t.Fatal(err)
	}
	if n := site.waits.Load(); n != 0 {
		t.Errorf("WaitPage called %d times for a listed site, want 0", n)
	}
	// an unlisted site keeps its own pacing
	if _, err := FetchChapterLimited(context.Background(), site, limitedChapter(server.URL), ratelimit.Open(t.TempDir(), nil, nil).Site("example.com"), func(int, int, error) {}); err != nil {
		t.Fatal(err)
	}
	if n := site.waits.Load(); n != 3 {
		t.Errorf("WaitPage called %d times for an unlisted site, want 3", n)
	}
}
