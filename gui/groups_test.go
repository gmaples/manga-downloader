// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

const (
	urlA = "https://a.example/manga/one"
	urlB = "https://b.example/manga/two"
)

// newTwoSeries returns a controller whose loads identify two series, A at
// urlA (2 chapters) and B at urlB (3 chapters), each a site of its own as core.Identify makes them. A
// identify gives a new site object each time, as reloading a URL does.
func newTwoSeries(t *testing.T, gateA, gateB chan struct{}) (c *Controller, sites func(url string) *fakeSite, dir string) {
	t.Helper()
	c, a, dir := newTestController(t, false, gateA, chapters(2, 1)...)
	a.title = "Series A"
	made := map[string]*fakeSite{}
	c.identify = func(url string, s *grabber.Settings) (grabber.Site, []error) {
		site := &fakeSite{
			Grabber:  &grabber.Grabber{URL: url, Settings: s},
			server:   a.server,
			chapters: chapters(2, 1),
			entered:  make(chan struct{}, 16),
			title:    "Series A",
			gate:     gateA,
		}
		if url == urlB {
			site.title, site.gate, site.chapters = "Series B", gateB, chapters(3, 1)
		}
		made[url] = site
		return site, nil
	}
	return c, func(url string) *fakeSite { return made[url] }, dir
}

// startAll loads url, selects every chapter and starts a group
func startAll(t *testing.T, c *Controller, url string) {
	t.Helper()
	if err := c.Load(url); err != nil {
		t.Fatalf("Load(%s) = %v", url, err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start(%s) = %v", url, err)
	}
}

// groupNamed is the state of the group titled title
func groupNamed(t *testing.T, c *Controller, title string) GroupState {
	t.Helper()
	for _, g := range c.Groups() {
		if g.Title == title {
			return g
		}
	}
	t.Fatalf("no group %q in %+v", title, c.Groups())
	return GroupState{}
}

// waitGroup polls until the group titled title has a summary
func waitGroup(t *testing.T, c *Controller, title string) *Summary {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s := groupNamed(t, c, title).Summary; s != nil {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("group %q did not finish", title)
	return nil
}

// cbzNamed lists the CBZs in dir whose name starts with prefix
func cbzNamed(t *testing.T, dir, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".cbz") {
			out = append(out, e.Name())
		}
	}
	return out
}

// A second series loads and downloads while the first group still runs, and
// each group ends with its own summary
func TestSecondSeriesRunsBesideTheFirst(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, dir := newTwoSeries(t, gateA, nil)

	startAll(t, c, urlA)
	<-sites(urlA).entered
	startAll(t, c, urlB)

	b := waitGroup(t, c, "Series B")
	if b.Saved != 3 || b.Cancelled || len(b.Failures) != 0 {
		t.Errorf("B's summary = %+v, want 3 saved", b)
	}
	if got := cbzNamed(t, dir, "Series B"); len(got) != 3 {
		t.Errorf("B's files = %v, want 3", got)
	}
	a := groupNamed(t, c, "Series A")
	if !a.Running || a.Summary != nil || !c.Running() {
		t.Errorf("A = %+v, running %v; want A still running while B finished", a, c.Running())
	}
	if s := c.LastSummary(); s != nil {
		t.Errorf("LastSummary while A runs = %+v, want nil", s)
	}

	close(gateA)
	if s := waitGroup(t, c, "Series A"); s.Saved != 2 {
		t.Errorf("A's summary = %+v, want 2 saved", s)
	}
	waitFinished(t, c)
	if s := c.LastSummary(); s == nil || s.Saved != 2 {
		t.Errorf("LastSummary = %+v, want A's, which ended last", s)
	}
	groups := c.Groups()
	if len(groups) != 2 || groups[0].Title != "Series A" || groups[1].Title != "Series B" || groups[0].ID == groups[1].ID {
		t.Errorf("groups = %+v, want A then B, with their own ids", groups)
	}
	if len(groups[0].Jobs) != 2 || len(groups[1].Jobs) != 3 {
		t.Errorf("rows = %d and %d, want 2 and 3", len(groups[0].Jobs), len(groups[1].Jobs))
	}
}

// A series never downloads twice at once: neither from the same load nor
// from a reload of its URL, which gives it a new site
func TestStartRefusesASeriesAlreadyDownloading(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, _ := newTwoSeries(t, gateA, nil)
	startAll(t, c, urlA)
	<-sites(urlA).entered

	busy := func(what string) {
		t.Helper()
		err := c.Start()
		if m := messageOf(err); m == nil || m.Key != "errors.seriesBusy" {
			t.Errorf("Start %s = %v, want errors.seriesBusy", what, err)
		}
	}
	busy("again")
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	c.SetAll(true)
	busy("after a reload")
	if n := len(c.Groups()); n != 1 {
		t.Errorf("%d groups, want the 1 running", n)
	}

	close(gateA)
	waitGroup(t, c, "Series A")
	if err := c.Start(); err != nil {
		t.Errorf("Start once A ended = %v, want it to start", err)
	}
	waitFinished(t, c)
}

// CancelGroup stops its group only, and Cancel every one still running
func TestCancelGroup(t *testing.T) {
	gateA, gateB := make(chan struct{}), make(chan struct{})
	c, sites, _ := newTwoSeries(t, gateA, gateB)
	c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 1 })
	startAll(t, c, urlA)
	<-sites(urlA).entered
	startAll(t, c, urlB)
	<-sites(urlB).entered

	c.CancelGroup(groupNamed(t, c, "Series A").ID)
	if a, b := groupNamed(t, c, "Series A"), groupNamed(t, c, "Series B"); !a.Cancelling || b.Cancelling {
		t.Errorf("cancelling A %v, B %v; want A only", a.Cancelling, b.Cancelling)
	}
	if c.Cancelling() {
		t.Error("Cancelling() with B still going, want false")
	}
	close(gateA)
	if s := waitGroup(t, c, "Series A"); !s.Cancelled || s.Saved != 1 {
		t.Errorf("A = %+v, want cancelled with the 1 chapter in flight saved", s)
	}

	c.Cancel()
	if !c.Cancelling() {
		t.Error("Cancelling() after Cancel, want true")
	}
	close(gateB)
	if s := waitGroup(t, c, "Series B"); !s.Cancelled || s.Saved != 1 {
		t.Errorf("B = %+v, want cancelled with the 1 chapter in flight saved", s)
	}
}

// Clear drops the groups that ended and keeps the running ones, and leaves
// the browser to them
func TestClearKeepsRunningGroups(t *testing.T) {
	gateB := make(chan struct{})
	c, sites, _ := newTwoSeries(t, nil, gateB)
	var closed atomic.Int32
	c.closeBrowser = func() { closed.Add(1) }

	startAll(t, c, urlA)
	waitGroup(t, c, "Series A")
	startAll(t, c, urlB)
	<-sites(urlB).entered

	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	groups := c.Groups()
	if len(groups) != 1 || groups[0].Title != "Series B" || !groups[0].Running {
		t.Errorf("groups = %+v, want B alone, running", groups)
	}
	if c.Loaded() {
		t.Error("Clear left the series loaded")
	}
	time.Sleep(50 * time.Millisecond)
	if n := closed.Load(); n != 0 {
		t.Errorf("Clear closed the browser %d times while B ran", n)
	}

	close(gateB)
	waitGroup(t, c, "Series B")
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	c.closing.Wait()
	if len(c.Groups()) != 0 || closed.Load() != 1 {
		t.Errorf("after the last group: %d groups, browser closed %d times; want none and once", len(c.Groups()), closed.Load())
	}
}

// A listing cached for a URL whose site a running group uses isn't served:
// serving it would point that site at the new load's settings
func TestCachedListingOfARunningSiteIsNotReused(t *testing.T) {
	c, sites, _ := newTwoSeries(t, nil, nil)
	s := c.Settings()
	listing, site, err := c.listingFor(urlA, &s, nil, false)
	if err != nil || site == nil {
		t.Fatalf("listingFor = %v, %v", site, err)
	}
	if again, _, _ := c.listingFor(urlA, &s, nil, true); again != listing {
		t.Fatal("the listing wasn't cached, so this test proves nothing")
	}

	g, _ := attach(c)
	c.mu.Lock()
	g.site, g.running = site, true
	c.mu.Unlock()
	other := s
	_, got, err := c.listingFor(urlA, &other, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == site {
		t.Error("a site a running group uses was served from the cache")
	}
	if got != grabber.Site(sites(urlA)) {
		t.Error("the URL wasn't identified again")
	}
	if site.(*fakeSite).Settings != &s {
		t.Error("the running group's site was pointed at other settings")
	}
}
