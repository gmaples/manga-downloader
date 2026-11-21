// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// mirrorServer answers each path with a 400 for the paths in bad (as
// mangalib's main image server does for its "1..jpg" pages), and with its
// name and the path otherwise; it counts the requests per path
func mirrorServer(t *testing.T, name string, bad ...string) (*httptest.Server, func(string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		for _, b := range bad {
			if r.URL.Path == b {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
		}
		_, _ = w.Write([]byte(name + r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	return srv, func(p string) int {
		mu.Lock()
		defer mu.Unlock()
		return hits[p]
	}
}

func hostPort(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func mirrorSite(url string) fetchChapterTestSite {
	return fetchChapterTestSite{Grabber: &grabber.Grabber{URL: url, Settings: &grabber.Settings{
		MaxConcurrency: grabber.MaxConcurrency{Chapters: 1, Pages: 3},
		Retry:          1,
	}}}
}

// A page the main server fails is fetched from the hosts site-mirrors.txt
// names for it, in order, with the same path; the other pages stay on the main
// server
func TestFetchChapterLimited_TriesMirrors(t *testing.T) {
	main, mainHits := mirrorServer(t, "main", "/c/1..jpg")
	dead, deadHits := mirrorServer(t, "dead", "/c/1..jpg")
	second, secondHits := mirrorServer(t, "second")
	root := t.TempDir()
	line := hostPort(t, main.URL) + ": " + hostPort(t, dead.URL) + ", " + hostPort(t, second.URL) + "\n"
	if err := os.WriteFile(filepath.Join(root, ratelimit.MirrorsFileName), []byte("# comment\n"+line), 0o644); err != nil {
		t.Fatal(err)
	}
	lim := ratelimit.Open(root, nil, nil).Site("example.com")
	chapter := &grabber.Chapter{Number: 1, Pages: []grabber.Page{
		{Number: 1, URL: main.URL + "/c/1..jpg"},
		{Number: 2, URL: main.URL + "/c/2.jpg"},
	}}

	files, err := FetchChapterLimited(context.Background(), mirrorSite(main.URL), chapter, lim, func(int, int, error) {})
	if err != nil {
		t.Fatalf("FetchChapterLimited: %v", err)
	}
	if got := string(files[0].Data); got != "second/c/1..jpg" {
		t.Errorf("page 1 = %q, want it from the second mirror", got)
	}
	if got := string(files[1].Data); got != "main/c/2.jpg" {
		t.Errorf("page 2 = %q, want it from the main server", got)
	}
	// each server gets the page's own retries: 1 retry = 2 requests
	if mainHits("/c/1..jpg") != 2 || deadHits("/c/1..jpg") != 2 || secondHits("/c/1..jpg") != 1 {
		t.Errorf("requests for page 1: main %d, dead %d, second %d; want 2, 2, 1",
			mainHits("/c/1..jpg"), deadHits("/c/1..jpg"), secondHits("/c/1..jpg"))
	}
	if secondHits("/c/2.jpg") != 0 {
		t.Errorf("page 2 went to the mirror %d times, want none", secondHits("/c/2.jpg"))
	}
}

// Without a site-mirrors.txt line for its host, a failing page fails the
// chapter as before, and no other server is asked
func TestFetchChapterLimited_NoMirrorLine(t *testing.T) {
	main, _ := mirrorServer(t, "main", "/c/1..jpg")
	other, otherHits := mirrorServer(t, "other")
	root := t.TempDir()
	// a line for another host only
	if err := os.WriteFile(filepath.Join(root, ratelimit.MirrorsFileName), []byte("img2.imglib.info: "+hostPort(t, other.URL)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lim := ratelimit.Open(root, nil, nil).Site("example.com")
	chapter := &grabber.Chapter{Number: 1, Pages: []grabber.Page{{Number: 1, URL: main.URL + "/c/1..jpg"}}}

	_, err := FetchChapterLimited(context.Background(), mirrorSite(main.URL), chapter, lim, func(int, int, error) {})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v, want the main server's 400", err)
	}
	if otherHits("/c/1..jpg") != 0 {
		t.Errorf("an unlisted host's page went to another server")
	}
}
