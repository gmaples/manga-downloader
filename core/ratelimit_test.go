// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/ratelimit"
)

// A 429 to a chapter's page list and one to a page are both sat out and
// fetched again, the chapter comes out whole, and the site's gap is saved in
// the folder picked, not in the series folder inside it
func TestDownloadSitsOut429sAndSavesTheLimit(t *testing.T) {
	g, mg, w, mw := ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait
	ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait = 2*time.Millisecond, time.Second, 5*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { ratelimit.StartGap, ratelimit.MaxGap, ratelimit.StartWait, ratelimit.MaxWait = g, mg, w, mw })

	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 3, limitFetch: map[int]bool{1: true}},
		fakeChapter{number: 2, pages: 3, limitedOnce: map[int]bool{1: true}},
	)
	series, root := newSeries(t, site, false)
	dir := SeriesDir(root, series.Title, "")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	if _, err := series.WithOutputDir(dir).Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if n := rec.count("download-failed") + rec.count("fetch-failed"); n != 0 {
		t.Errorf("%d chapters failed: %q", n, rec.events)
	}
	want := []string{
		filepath.Join(dir, "Test Series 1 - Chapter 1.cbz"),
		filepath.Join(dir, "Test Series 2 - Chapter 2.cbz"),
	}
	if files := cbzIn(t, dir); !slices.Equal(files, want) {
		t.Fatalf("files = %q, want %q", files, want)
	}
	for _, f := range want {
		var pages int
		for _, e := range zipEntries(t, f) {
			if strings.HasSuffix(e, ".png") {
				pages++
			}
		}
		if pages != 3 {
			t.Errorf("%s holds %d pages, want 3", filepath.Base(f), pages)
		}
	}
	if site.served[1] != 2 {
		t.Errorf("chapter 1's page list was fetched %d times, want 2", site.served[1])
	}

	data, err := os.ReadFile(filepath.Join(root, ratelimit.FileName))
	if err != nil {
		t.Fatalf("no %s in the folder picked: %v", ratelimit.FileName, err)
	}
	if !strings.Contains(string(data), "\nexample.com: ") {
		t.Errorf("file =\n%s\nwant an example.com line", data)
	}
	if _, err := os.Stat(filepath.Join(dir, ratelimit.FileName)); !os.IsNotExist(err) {
		t.Errorf("the file was written into the series folder too: %v", err)
	}
}
