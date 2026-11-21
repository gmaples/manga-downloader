// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

func wantMarks(t *testing.T, c *Controller, want ...string) {
	t.Helper()
	if got := c.ChapterMarks(); !slices.Equal(got, want) {
		t.Errorf("marks = %q, want %q", got, want)
	}
}

// Nothing loaded, nothing marked
func TestMarksNilWithoutASeries(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	if got := c.ChapterMarks(); got != nil {
		t.Errorf("marks = %q, want nil", got)
	}
}

// A chapter whose file is in the folder, under the name Skip existing looks
// for, is done before anything downloads; the others have no mark
func TestMarksFindTheFilesInTheFolder(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(3, 1)...)
	name, err := packer.ChapterName("Test Series", "{{.Series}} {{.Number}} - {{.Title}}", "cbz", &grabber.Chapter{Number: 2, Title: "Chapter 2"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	wantMarks(t, c, "", "done", "")

	// another format names other files, so it looks again
	if err := c.Configure(func(s *grabber.Settings) { s.Format = "epub" }); err != nil {
		t.Fatal(err)
	}
	wantMarks(t, c, "", "", "")
}

// A finished run marks what it saved done and what failed failed
func TestMarksAfterARun(t *testing.T) {
	list := chapters(3, 2)
	list[1].fail = map[int]bool{1: true}
	c, _, _ := newTestController(t, false, nil, list...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	wantMarks(t, c, "done", "failed", "done")
}

// While a run goes on, its chapter in flight is downloading and the rest
// wait; a cancel leaves the ones it never started cancelled
func TestMarksWhileRunningAndCancelled(t *testing.T) {
	gate := make(chan struct{})
	c, site, _ := newTestController(t, false, gate, chapters(4, 1)...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.MaxConcurrency.Chapters = 1 }); err != nil {
		t.Fatal(err)
	}
	// chapter 4 is left out of the run
	for i := range 3 {
		if err := c.SetSelected(i, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-site.entered
	wantMarks(t, c, "downloading", "queued", "queued", "")

	c.Cancel()
	close(gate)
	waitFinished(t, c)
	wantMarks(t, c, "done", "cancelled", "cancelled", "")
}

// The group's marks belong to its series: another series loaded in the
// window gets none of them
func TestMarksOnlyOfTheLoadedSeries(t *testing.T) {
	c, _, _ := newTwoSeries(t, nil, nil)
	startAll(t, c, urlA)
	waitGroup(t, c, "Series A")
	wantMarks(t, c, "done", "done")

	if err := c.Load(urlB); err != nil {
		t.Fatal(err)
	}
	wantMarks(t, c, "", "", "")
}

// A failed chapter of a volume stays failed though its volume was packed
// without it
func TestMarkFailedBeatsDone(t *testing.T) {
	g := newGroup()
	g.done["k"], g.failed["k"] = true, true
	if got := g.markOf("k"); got != MarkFailed {
		t.Errorf("mark = %q, want failed", got)
	}
}
