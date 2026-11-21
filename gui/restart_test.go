// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

// restartRow is the row of group g titled title, failing the test without one
func restartRow(t *testing.T, g GroupState, title string) Job {
	t.Helper()
	for _, j := range g.Jobs {
		if j.Title == title {
			return j
		}
	}
	t.Fatalf("no row %q in %+v", title, g.Jobs)
	return Job{}
}

// waitIdle polls until no group runs
func waitIdle(t *testing.T, c *Controller) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if !c.Running() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("downloads did not end")
}

// A failed row of an ended download restarts its chapter alone, at once, as
// a new download, and the list's marks follow
func TestRestartAFailedRow(t *testing.T) {
	list := chapters(3, 2)
	list[1].fail = map[int]bool{1: true}
	c, _, _ := newTestController(t, false, nil, list...)
	startAll(t, c, testURL)
	waitFinished(t, c)

	g := lastGroup(c)
	if g.Unfinished != 1 {
		t.Fatalf("Unfinished = %d, want the 1 failed chapter", g.Unfinished)
	}
	failed := restartRow(t, g, "Chapter 2")
	if !failed.Restart || failed.Key == "" {
		t.Fatalf("failed row = %+v, want it restartable with a key", failed)
	}
	if done := restartRow(t, g, "Chapter 1"); done.Restart {
		t.Errorf("a saved row is restartable: %+v", done)
	}

	url, load, err := c.Restart(g.ID, failed.Key)
	if err != nil || load || url != testURL {
		t.Fatalf("Restart = %q, %v, %v; want the loaded series started now", url, load, err)
	}
	if got := fmt.Sprint(c.SelectedIndices()); got != "[1]" {
		t.Errorf("selection = %s, want just the restarted chapter", got)
	}
	waitIdle(t, c)
	groups := c.Groups()
	if len(groups) != 2 || groups[1].Run.Chapters != 1 {
		t.Fatalf("groups = %+v, want a second download of 1 chapter", groups)
	}
	// it fails again (the page still fails), and the newer download says so;
	// the first one no longer offers it, as a newer one took it
	if groups[0].Unfinished != 0 || restartRow(t, groups[0], "Chapter 2").Restart {
		t.Errorf("first download still offers the restarted chapter: %+v", groups[0])
	}
	if groups[1].Unfinished != 1 {
		t.Errorf("second download's Unfinished = %d, want 1", groups[1].Unfinished)
	}
	wantMarks(t, c, "done", "failed", "done")
}

// Restarting a cancelled download's unfinished chapters downloads every
// chapter it never saved, rows or not
func TestRestartTheUnfinishedOfACancelledDownload(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(4, 1)...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 1 }); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-site.entered
	c.Cancel()
	close(gate)
	waitFinished(t, c)

	g := lastGroup(c)
	if g.Unfinished != 3 {
		t.Fatalf("Unfinished = %d, want the 3 cancelled chapters", g.Unfinished)
	}
	site.gate = nil
	if _, load, err := c.Restart(g.ID, ""); err != nil || load {
		t.Fatalf("Restart = %v, load %v", err, load)
	}
	waitIdle(t, c)
	wantMarks(t, c, "done", "done", "done", "done")
	if _, _, err := c.Restart(g.ID, ""); err == nil {
		t.Error("a second Restart with nothing left was not refused")
	}
}

// A restored download of another series loads it, selects its chapters to
// do and starts them once loaded; the restored group gives way to the new one
func TestRestartARestoredDownloadOfAnotherSeries(t *testing.T) {
	c, _, _ := newTwoSeries(t, nil, nil)
	label := grabber.ChapterLabel(listing{number: 2, title: "Chapter 2"})
	g, _ := attach(c)
	c.mu.Lock()
	g.restored, g.url, g.title = true, urlA, "Series A"
	g.jobs = []*Job{
		{Title: grabber.ChapterLabel(listing{number: 1, title: "Chapter 1"}), State: JobDone, Key: "todo:x"},
		{Title: label, State: JobTodo, Key: "todo:" + label},
	}
	c.mu.Unlock()
	if err := c.Load(urlB); err != nil {
		t.Fatal(err)
	}
	if st := c.Groups()[0]; st.Unfinished != 1 || !st.Jobs[1].Restart || st.Jobs[0].Restart {
		t.Fatalf("restored group = %+v, want its 1 to-do row restartable", st)
	}

	url, load, err := c.Restart(g.id, "todo:"+label)
	if err != nil || !load || url != urlA {
		t.Fatalf("Restart = %q, %v, %v; want urlA to load", url, load, err)
	}
	// what App.Restart does with it
	if err := c.Load(url); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, c)
	groups := c.Groups()
	if len(groups) != 1 || groups[0].Restored || groups[0].Run.Chapters != 1 || groups[0].URL != urlA {
		t.Fatalf("groups = %+v, want one download of chapter 2 of A in place of the restored one", groups)
	}
	wantMarks(t, c, "", "done")
}

// A failed chapter of a running download joins its queue again: its old row
// and failure go, so the summary counts the second try only
func TestRestartInARunningDownload(t *testing.T) {
	list := chapters(2, 1)
	list[0].fail = map[int]bool{0: true}
	c, site, _ := newTestController(t, false, nil, list...)
	site.pageGate, site.pageGated = make(chan struct{}), 2
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 1 }); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "chapter 1 to fail and chapter 2 to start", func() bool {
		return slices.Equal(c.ChapterMarks(), []string{"failed", "downloading"})
	})
	g := lastGroup(c)
	row := restartRow(t, g, "Chapter 1")
	if _, load, err := c.Restart(g.ID, row.Key); err != nil || load {
		t.Fatalf("Restart = %v, load %v", err, load)
	}
	st := lastGroup(c)
	if len(c.Groups()) != 1 || st.Run.Chapters != 2 {
		t.Fatalf("groups = %+v, want the running one with chapter 1 queued again", c.Groups())
	}
	for _, j := range st.Jobs {
		if j.Title == "Chapter 1" && j.State == JobFailed {
			t.Errorf("the old failed row stayed: %+v", st.Jobs)
		}
	}
	wantMarks(t, c, "queued", "downloading")
	close(site.pageGate)
	s := waitFinished(t, c)
	if len(s.Failures) != 1 || s.Saved != 1 {
		t.Errorf("summary = %+v, want chapter 2 saved and chapter 1's second try as the one failure", s)
	}
}
