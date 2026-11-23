// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// Delete series says what it takes, moves the series' folder to the trash
// with its count, logs "Deleted series", and drops the series from the
// history; the marks find no file afterwards
func TestDeleteSeries(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	c, _, root := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Configure(func(s *grabber.Settings) { s.SeriesFolders = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.DeleteInfo(); err == nil {
		t.Error("DeleteInfo without a series was not refused")
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
	wantMarks(t, c, "done", "done")
	if len(c.History()) == 0 {
		t.Fatal("the download left no history entry")
	}

	info, err := c.DeleteInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Whole || info.Files != 2 || filepath.Dir(info.Dir) != root {
		t.Fatalf("info = %+v, want the series folder in %s, whole, 2 files", info, root)
	}
	var steps []core.DeleteProgress
	if err := c.DeleteSeries(func(p core.DeleteProgress) { steps = append(steps, p) }); err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].Done != 2 || steps[0].Total != 2 {
		t.Errorf("progress = %+v", steps)
	}
	if _, err := os.Stat(info.Dir); !os.IsNotExist(err) {
		t.Errorf("the series folder is still there: %v", err)
	}
	if got, _ := os.ReadDir(filepath.Join(data, "Trash", "files")); len(got) != 2 {
		t.Errorf("trash holds %d files, want 2", len(got))
	}
	for _, e := range c.History() {
		if e.URL == testURL {
			t.Error("the series is still in the history")
		}
	}
	wantMarks(t, c, "", "")
	found := false
	for _, l := range c.Log() {
		if strings.HasPrefix(l.Text, "Deleted series Test Series from "+info.Dir) {
			found = true
		}
	}
	if !found {
		t.Error(`no "Deleted series" line in the log`)
	}
	if _, err := c.DeleteInfo(); err == nil {
		t.Error("a second delete with nothing left was not refused")
	}
}
