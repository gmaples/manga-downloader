// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// waitBars polls until ok holds for the site bars, and returns them
func waitBars(t *testing.T, c *Controller, what string, ok func([]SiteBar) bool) []SiteBar {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if bars := c.SiteBars(); ok(bars) {
			return bars
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s: bars = %+v", what, c.SiteBars())
	return nil
}

// Each site downloading has a bar, in the order its group started, with the
// chapter it's fetching
func TestSiteBarPerSite(t *testing.T) {
	gateA, gateB := make(chan struct{}), make(chan struct{})
	c, sites, _ := newTwoSeries(t, gateA, gateB)
	if bars := c.SiteBars(); len(bars) != 0 {
		t.Fatalf("bars before any download = %+v, want none", bars)
	}
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 1)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered
	startAll(t, c, urlB)
	<-sites(urlB).entered

	bars := c.SiteBars()
	if len(bars) != 2 || bars[0].Site != "a.example" || bars[1].Site != "b.example" {
		t.Fatalf("bars = %+v, want a.example then b.example", bars)
	}
	if bars[0].Title != "Chapter 2" || bars[0].Total != 0 {
		t.Errorf("a.example's bar = %+v, want chapter 2 fetching its page list", bars[0])
	}

	close(gateA)
	waitGroup(t, c, "Series A")
	bars = c.SiteBars()
	if len(bars) != 1 || bars[0].Site != "b.example" {
		t.Errorf("bars once A ended = %+v, want b.example's only", bars)
	}
	close(gateB)
	waitFinished(t, c)
	waitGroup(t, c, "Series B")
	if bars := c.SiteBars(); len(bars) != 0 {
		t.Errorf("bars once everything ended = %+v, want none", bars)
	}
}

// Two series from one site share its bar, which shows the chapter that
// started first
func TestSiteBarSharedBySite(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, _ := newTwoSeries(t, gateA, nil)
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 1)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered
	const other = "https://a.example/manga/other"
	if err := c.Load(other); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 0)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(other).entered

	bars := c.SiteBars()
	if len(c.Groups()) != 2 || len(bars) != 1 || bars[0].Site != "a.example" || bars[0].Title != "Chapter 2" {
		t.Errorf("%d groups, bars = %+v; want 2 groups sharing one bar on the first group's chapter 2", len(c.Groups()), bars)
	}
	close(gateA)
	for _, g := range c.Groups() {
		waitGroup(t, c, g.Title)
	}
}

// A chapter's bar counts its pages, and a 429's pause shows on its site's bar
func TestSiteBarProgressAndPause(t *testing.T) {
	start := ratelimit.StartWait
	ratelimit.StartWait = 300 * time.Millisecond
	t.Cleanup(func() { ratelimit.StartWait = start })

	c, site, dir := newTestController(t, false, nil, chapters(1, 3)...)
	site.pageGate, site.pageGated = make(chan struct{}), 1
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	bars := waitBars(t, c, "downloading", func(b []SiteBar) bool { return len(b) == 1 && b[0].Total > 0 })
	want := int64(3 * (core.DownloadWeight + core.ArchiveWeight))
	if b := bars[0]; b.Site != "example.com" || b.Title != "Chapter 1" || b.Total != want || b.Done != 0 || !b.Until.IsZero() {
		t.Errorf("bar = %+v, want chapter 1 at 0 of %d, not paused", b, want)
	}

	// the download's book is the one of its folder, shared while it runs
	book, release := ratelimit.Share(dir, nil, nil)
	book.Site("example.com").Hit(0, 0)
	release()
	if b := c.SiteBars()[0]; b.Until.IsZero() || time.Until(b.Until) > 300*time.Millisecond {
		t.Errorf("bar after a 429 = %+v, want paused for up to 300ms", b)
	}

	close(site.pageGate)
	if s := waitFinished(t, c); s.Saved != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if bars := c.SiteBars(); len(bars) != 0 {
		t.Errorf("bars after the run = %+v, want none", bars)
	}
}

// The bar follows its chapter through the events: the page list, each page,
// each archived page, and its end, in a chapters run and in a bundle, which
// has no archive steps per chapter
func TestSiteBarFollowsEvents(t *testing.T) {
	for _, bundle := range []bool{false, true} {
		c := NewController(&grabber.Settings{}, nil)
		g, e := attach(c)
		c.mu.Lock()
		g.running, g.siteKey = true, "example.com"
		if bundle {
			g.bundle = &Job{}
		}
		c.mu.Unlock()
		bar := func() SiteBar {
			t.Helper()
			bars := c.SiteBars()
			if len(bars) != 1 {
				t.Fatalf("bundle %v: bars = %+v, want one", bundle, bars)
			}
			return bars[0]
		}

		listed := &grabber.Chapter{Number: 1, Title: "One"}
		ch := &grabber.Chapter{Number: 1, Title: "One", PagesCount: 2}
		if b := bar(); b.Title != "" {
			t.Errorf("bundle %v: bar before any chapter = %+v, want between chapters", bundle, b)
		}
		e.Fetching(listed)
		if b := bar(); b.Title != "One" || b.Total != 0 {
			t.Errorf("bundle %v: bar fetching = %+v", bundle, b)
		}
		e.Fetched(listed, ch)
		e.Started(ch)
		e.Page(ch, 0, nil)
		e.Page(ch, 1, nil)
		total := int64(2 * core.DownloadWeight)
		if !bundle {
			total += 2 * core.ArchiveWeight
		}
		if b := bar(); b.Done != 2*core.DownloadWeight || b.Total != total {
			t.Errorf("bundle %v: bar after the pages = %+v, want %d of %d", bundle, b, 2*core.DownloadWeight, total)
		}
		if bundle {
			e.Downloaded(ch)
		} else {
			e.Archived(ch)
			if b := bar(); b.Done != 2*core.DownloadWeight+core.ArchiveWeight {
				t.Errorf("bar after an archived page = %+v", b)
			}
			e.Packed(ch, "one.cbz", nil)
		}
		if b := bar(); b.Title != "" {
			t.Errorf("bundle %v: bar after the chapter = %+v, want between chapters", bundle, b)
		}
	}
}
