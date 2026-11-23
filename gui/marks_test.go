// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

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

// Download missing takes every chapter not saved: the failed one and the one
// never picked, and then nothing is left; the selection is kept when refused
func TestDownloadMissing(t *testing.T) {
	list := chapters(4, 2)
	list[1].fail = map[int]bool{1: true}
	c, _, _ := newTestController(t, false, nil, list...)
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{0, 1, 2} {
		if err := c.SetSelected(i, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	wantMarks(t, c, "done", "failed", "done", "")

	if err := c.DownloadMissing(); err != nil {
		t.Fatal(err)
	}
	if got := c.SelectedIndices(); !slices.Equal(got, []int{1, 3}) {
		t.Errorf("selection = %v, want the failed chapter and the one never picked", got)
	}
	waitIdle(t, c)
	wantMarks(t, c, "done", "failed", "done", "done")

	list[1].fail = nil
	if err := c.DownloadMissing(); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, c)
	wantMarks(t, c, "done", "done", "done", "done")
	if err := c.DownloadMissing(); err == nil {
		t.Error("DownloadMissing with nothing missing was not refused")
	}
	if got := c.SelectedIndices(); !slices.Equal(got, []int{1}) {
		t.Errorf("selection = %v, want the last download's kept after the refusal", got)
	}
}

// A chapter whose number is in the folder under another name (another
// title or group, from an earlier download) is done too, and its note names
// the file
func TestMarksFindAChapterByItsNumber(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(3, 1)...)
	if err := os.WriteFile(filepath.Join(dir, "Test Series 2 - Chapter 2 Old title [Sherlyn].cbz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	wantMarks(t, c, "", "done", "")
	if got := c.ChapterNotes(); !slices.Equal(got, []string{"", "Test Series 2 - Chapter 2 Old title [Sherlyn].cbz", ""}) {
		t.Errorf("notes = %q", got)
	}
}

// With Skip existing, a download first moves the folder's duplicates to the
// trash, says so in the group's cleanup and the log, and leaves out the
// chapters whose number is already there
func TestDownloadCorrelatesTheFolder(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	c, _, dir := newTestController(t, false, nil, chapters(2, 1)...)
	for _, f := range []struct {
		name string
		age  time.Duration
	}{
		{"Test Series 1 - Chapter 1 Old [Sherlyn].cbz", 2 * time.Hour},
		{"Test Series 1 - Chapter 1.cbz", time.Hour},
	} {
		p := filepath.Join(dir, f.name)
		if err := os.WriteFile(p, []byte(f.name), 0o644); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(-f.age)
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
	}
	// Skip existing on in the window sets both (see applyOptions)
	if err := c.Configure(func(s *grabber.Settings) { s.SkipExisting, s.Dedupe = true, true }); err != nil {
		t.Fatal(err)
	}
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

	groups := c.Groups()
	g := groups[len(groups)-1]
	if g.Cleanup == nil || g.Cleanup.Removed != 1 || g.Cleanup.Failed != 0 {
		t.Fatalf("cleanup = %+v", g.Cleanup)
	}
	if _, err := os.Stat(filepath.Join(data, "Trash", "files", "Test Series 1 - Chapter 1.cbz")); err != nil {
		t.Errorf("duplicate not in the trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Test Series 1 - Chapter 1 Old [Sherlyn].cbz")); err != nil {
		t.Errorf("older file gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "Test Series 2 - Chapter 2.cbz")); err != nil {
		t.Errorf("chapter 2 not saved: %v", err)
	}
	wantMarks(t, c, "done", "done")
	found := false
	for _, l := range c.Log() {
		if l.Msg != nil && l.Msg.Key == "log.removedNumber" {
			found = true
		}
	}
	if !found {
		t.Error("no log line for the removed duplicate")
	}
}
