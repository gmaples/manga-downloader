// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// in a folder shared with other series only the series' chapter files go,
// whatever their title or group, and the count moves as it goes
func TestDeleteSeriesTakesOnlyItsFiles(t *testing.T) {
	bin := trashIn(t)
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	writeAt(t, dir, "Test Series 1 - Old title [Alpha].cbz", "one", time.Hour)
	writeAt(t, dir, "Test Series 2 - Two.cbz", "two", time.Hour)
	writeAt(t, dir, "Other Series 1 - One.cbz", "other", time.Hour)
	writeAt(t, dir, "notes.txt", "keep", time.Hour)

	plan, err := series.DeletePlan(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Names); got != 2 || plan.Bytes != 6 {
		t.Fatalf("plan %v, %d bytes; want the 2 series files, 6 bytes", plan.Names, plan.Bytes)
	}
	var steps []DeleteProgress
	r := plan.Run("", func(p DeleteProgress) { steps = append(steps, p) })
	if len(r.Moved) != 2 || len(r.Failed) != 0 || r.Bytes != 6 {
		t.Fatalf("result %+v", r)
	}
	if got := names(t, dir); got != "Other Series 1 - One.cbz|notes.txt" {
		t.Errorf("left %q", got)
	}
	if got := names(t, bin); got != "Test Series 1 - Old title [Alpha].cbz|Test Series 2 - Two.cbz" {
		t.Errorf("trash %q", got)
	}
	if len(steps) != 1 || steps[0] != (DeleteProgress{Done: 2, Total: 2}) {
		t.Errorf("progress %+v", steps)
	}
	if r.DirRemoved {
		t.Error("a shared folder must stay")
	}
}

// a series' own folder goes whole, folder included, and the command line's
// move puts the files under [Empty Me] instead, renaming a name taken there
func TestDeleteSeriesWholeFolderAndMove(t *testing.T) {
	site := newFakeSite(t)
	series, root := newSeries(t, site, false)
	dir := filepath.Join(root, "Test Series [en]")
	if err := os.MkdirAll(filepath.Join(dir, "raw chapter"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, dir, "Test Series 1 - One.cbz", "one", time.Hour)
	writeAt(t, dir, "cover.jpg", "jpg", time.Hour)
	writeAt(t, filepath.Join(dir, "raw chapter"), "001.jpg", "page", time.Hour)

	plan, err := series.DeletePlan(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Names); got != 3 || plan.Bytes != 10 {
		t.Fatalf("plan %v, %d bytes", plan.Names, plan.Bytes)
	}
	to := EmptyMeFolder(root, series.Title)
	if err := os.MkdirAll(to, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, to, "cover.jpg", "older", time.Hour)
	r := plan.Run(to, nil)
	if len(r.Moved) != 3 || len(r.Failed) != 0 || !r.DirRemoved {
		t.Fatalf("result %+v", r)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the series folder is still there: %v", err)
	}
	if got := names(t, to); got != "Test Series 1 - One.cbz|cover (2).jpg|cover.jpg|raw chapter" {
		t.Errorf("[Empty Me] holds %q", got)
	}
}

// nothing of the series, or a copy of the program writing into the folder,
// stops the delete before anything moves
func TestDeleteSeriesRefuses(t *testing.T) {
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	if _, err := series.DeletePlan(dir, false); !errors.Is(err, ErrNothingToDelete) {
		t.Errorf("empty folder: %v", err)
	}
	if _, err := series.DeletePlan(filepath.Join(dir, "none"), true); !errors.Is(err, ErrNothingToDelete) {
		t.Errorf("no folder: %v", err)
	}
	writeAt(t, dir, "Test Series 1 - One.cbz", "one", time.Hour)
	writeAt(t, dir, "Test Series 2 - Two.cbz"+ClaimSuffix, "", time.Hour)
	if _, err := series.DeletePlan(dir, false); !errors.Is(err, ErrSeriesBusy) {
		t.Errorf("claimed: %v", err)
	}
}
