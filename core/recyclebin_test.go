// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/trash"
)

// noBin makes the test's folders sit on a drive with no Recycle Bin
func noBin(t *testing.T) {
	t.Helper()
	was := hasRecycleBin
	hasRecycleBin = func(string) bool { return false }
	t.Cleanup(func() { hasRecycleBin = was })
}

// asker answers ConfirmPermanent with yes, and records what it was asked
type asker struct {
	NoEvents
	yes   bool
	asked int
	dir   string
	n     int
	bytes int64
}

func (a *asker) ConfirmPermanent(dir string, n int, bytes int64) bool {
	a.asked++
	a.dir, a.n, a.bytes = dir, n, bytes
	return a.yes
}

// on a drive with no Recycle Bin the dedupe asks once; no keeps every file
// (each duplicate failed with ErrNoRecycleBin, its number still had), yes
// deletes the duplicates for good
func TestCorrelateAsksWithoutRecycleBin(t *testing.T) {
	trashIn(t)
	noBin(t)
	for _, yes := range []bool{false, true} {
		site := newFakeSite(t)
		series, dir := newSeries(t, site, false)
		writeAt(t, dir, "Test Series 1 - A.cbz", "same", 3*time.Hour)
		writeAt(t, dir, "Test Series 1 - A v2.cbz", "same", 2*time.Hour)
		writeAt(t, dir, "Test Series 2 - B.cbz", "b", 3*time.Hour)
		writeAt(t, dir, "Test Series 2 - B v2.cbz", "bb", 2*time.Hour)
		ev := &asker{yes: yes}
		r, err := series.Correlate(dir, ev)
		if err != nil {
			t.Fatal(err)
		}
		if ev.asked != 1 || ev.dir != dir || ev.n != 2 || ev.bytes != 6 {
			t.Errorf("yes=%v: asked %d times, about %q, %d files, %d bytes", yes, ev.asked, ev.dir, ev.n, ev.bytes)
		}
		got := names(t, dir)
		if !yes {
			if got != "Test Series 1 - A v2.cbz|Test Series 1 - A.cbz|Test Series 2 - B v2.cbz|Test Series 2 - B.cbz" {
				t.Errorf("no: folder %q", got)
			}
			if r.Permanent || len(r.Removed) != 0 || len(r.Failed) != 2 {
				t.Errorf("no: permanent %v, removed %v, failed %v", r.Permanent, r.Removed, r.Failed)
			}
			for _, d := range r.Failed {
				if !errors.Is(d.Err, trash.ErrNoRecycleBin) {
					t.Errorf("no: %s failed with %v", d.Name, d.Err)
				}
			}
			if r.Have["1"] == "" || r.Have["2"] == "" {
				t.Errorf("no: have %v", r.Have)
			}
			continue
		}
		if got != "Test Series 1 - A.cbz|Test Series 2 - B.cbz" {
			t.Errorf("yes: folder %q", got)
		}
		if !r.Permanent || len(r.Removed) != 2 || r.RemovedBytes != 6 || len(r.Failed) != 0 {
			t.Errorf("yes: permanent %v, removed %v (%d bytes), failed %v", r.Permanent, r.Removed, r.RemovedBytes, r.Failed)
		}
	}
}

// with a Recycle Bin nothing is asked, and nothing is deleted for good
func TestCorrelateDoesntAskWithRecycleBin(t *testing.T) {
	bin := trashIn(t)
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	writeAt(t, dir, "Test Series 1 - A.cbz", "same", 3*time.Hour)
	writeAt(t, dir, "Test Series 1 - A v2.cbz", "same", 2*time.Hour)
	ev := &asker{yes: true}
	r, err := series.Correlate(dir, ev)
	if err != nil {
		t.Fatal(err)
	}
	if ev.asked != 0 || r.Permanent {
		t.Errorf("asked %d times, permanent %v", ev.asked, r.Permanent)
	}
	if _, err := os.Stat(filepath.Join(bin, "Test Series 1 - A v2.cbz")); err != nil {
		t.Errorf("not in the trash: %v", err)
	}
}

// Delete series on a drive with no Recycle Bin: the plan says so, Run
// without leave deletes nothing, with leave deletes for good; a move into a
// folder (the command line's [Empty Me]) never asks
func TestDeleteRunWithoutRecycleBin(t *testing.T) {
	trashIn(t)
	noBin(t)
	for _, tc := range []struct {
		name  string
		to    bool
		allow bool
		left  string
	}{
		{"refused", false, false, "Test Series 1 - A.cbz|Test Series 2 - B.cbz"},
		{"for good", false, true, ""},
		{"empty me", true, false, ""},
	} {
		site := newFakeSite(t)
		series, dir := newSeries(t, site, false)
		writeAt(t, dir, "Test Series 1 - A.cbz", "a", time.Hour)
		writeAt(t, dir, "Test Series 2 - B.cbz", "b", time.Hour)
		plan, err := series.DeletePlan(dir, false)
		if err != nil {
			t.Fatal(err)
		}
		if !plan.Permanent {
			t.Errorf("%s: plan.Permanent is false", tc.name)
		}
		to := ""
		if tc.to {
			to = filepath.Join(t.TempDir(), "empty me")
		}
		r := plan.Run(to, tc.allow, nil)
		if got := names(t, dir); got != tc.left {
			t.Errorf("%s: folder %q, want %q", tc.name, got, tc.left)
		}
		if tc.left != "" {
			for n, err := range r.Failed {
				if !errors.Is(err, trash.ErrNoRecycleBin) {
					t.Errorf("%s: %s failed with %v", tc.name, n, err)
				}
			}
			if len(r.Failed) != 2 {
				t.Errorf("%s: failed %v", tc.name, r.Failed)
			}
		} else if len(r.Failed) != 0 || len(r.Moved) != 2 {
			t.Errorf("%s: moved %v, failed %v", tc.name, r.Moved, r.Failed)
		}
		if r.Permanent != (tc.allow && !tc.to) {
			t.Errorf("%s: result permanent %v", tc.name, r.Permanent)
		}
	}
}
