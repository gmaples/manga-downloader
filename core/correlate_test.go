// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeAt writes a file holding data, last modified age ago
func writeAt(t *testing.T, dir, name, data string, age time.Duration) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age).Truncate(time.Second)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// names lists a folder
func names(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return strings.Join(out, "|")
}

// trashIn points the trash at a temporary folder, as the freedesktop.org
// trash of the test's system, and returns the folder files go to
func trashIn(t *testing.T) string {
	t.Helper()
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	return filepath.Join(data, "Trash", "files")
}

// the same bytes under any name are one chapter, and a chapter number that's
// in the folder twice is one chapter whatever its title and group: the oldest
// file stays and the others go to the trash
func TestCorrelateRemovesDuplicates(t *testing.T) {
	bin := trashIn(t)
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	writeAt(t, dir, "Test Series 1 - Chapter 1 Old title [Alpha].cbz", "one", 3*time.Hour)
	writeAt(t, dir, "Test Series 1 - New title.cbz", "one-new", time.Hour)
	writeAt(t, dir, "Test Series 2 - Two.cbz", "two", 3*time.Hour)
	writeAt(t, dir, "Test Series 2 - Two v2.cbz", "two", time.Hour)
	writeAt(t, dir, "copy of two.cbz", "two", time.Hour)
	writeAt(t, dir, "Test Series 3 - Three.cbz", "three", time.Hour)
	// two chapter numbers with the same bytes (a site's placeholder) stay apart
	writeAt(t, dir, "Test Series 4 - Locked.cbz", "locked", 2*time.Hour)
	writeAt(t, dir, "Test Series 5 - Locked.cbz", "locked", time.Hour)
	// not chapters of the series, or not chapter files: left alone
	writeAt(t, dir, "Test Series 1-50.cbz", "bundle", time.Hour)
	writeAt(t, dir, "Test Series Vol. 01 (1-2).cbz", "volume", time.Hour)
	writeAt(t, dir, "Other 1 - One.cbz", "other", time.Hour)
	writeAt(t, dir, "Test Series 1 - notes.txt", "one", time.Hour)

	r, err := series.Correlate(dir)
	if err != nil {
		t.Fatalf("Correlate: %v", err)
	}

	var removed []string
	for _, d := range r.Removed {
		removed = append(removed, d.Name+" ~ "+d.Kept)
	}
	want := "Test Series 1 - New title.cbz ~ Test Series 1 - Chapter 1 Old title [Alpha].cbz|" +
		"Test Series 2 - Two v2.cbz ~ Test Series 2 - Two.cbz|" +
		"copy of two.cbz ~ Test Series 2 - Two.cbz"
	if got := strings.Join(removed, "|"); got != want {
		t.Errorf("removed:\n got %q\nwant %q", got, want)
	}
	if r.RemovedBytes != int64(len("one-new")+2*len("two")) {
		t.Errorf("RemovedBytes = %d", r.RemovedBytes)
	}
	if len(r.Failed) != 0 {
		t.Errorf("failed: %v", r.Failed)
	}
	if got, want := names(t, dir), "Other 1 - One.cbz|Test Series 1 - Chapter 1 Old title [Alpha].cbz|Test Series 1 - notes.txt|Test Series 1-50.cbz|Test Series 2 - Two.cbz|Test Series 3 - Three.cbz|Test Series 4 - Locked.cbz|Test Series 5 - Locked.cbz|Test Series Vol. 01 (1-2).cbz"; got != want {
		t.Errorf("folder:\n got %q\nwant %q", got, want)
	}
	if got, want := names(t, bin), "Test Series 1 - New title.cbz|Test Series 2 - Two v2.cbz|copy of two.cbz"; got != want {
		t.Errorf("trash:\n got %q\nwant %q", got, want)
	}
	if r.Have["1"] != "Test Series 1 - Chapter 1 Old title [Alpha].cbz" || r.Have["2"] != "Test Series 2 - Two.cbz" || r.Have["3"] != "Test Series 3 - Three.cbz" || len(r.Have) != 5 {
		t.Errorf("Have = %v", r.Have)
	}
}

// a file another copy of the program is writing is left alone
func TestCorrelateLeavesClaimedFiles(t *testing.T) {
	trashIn(t)
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	writeAt(t, dir, "Test Series 1 - A.cbz", "a", 2*time.Hour)
	writeAt(t, dir, "Test Series 1 - B.cbz", "b", time.Hour)
	writeAt(t, dir, "Test Series 1 - B.cbz"+ClaimSuffix, "claim", 0)

	r, err := series.Correlate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Removed) != 0 {
		t.Errorf("removed %v", r.Removed)
	}
}

// a missing folder has nothing to correlate
func TestCorrelateMissingFolder(t *testing.T) {
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	r, err := series.Correlate(filepath.Join(dir, "nope"))
	if err != nil || len(r.Have) != 0 {
		t.Errorf("got %v, %v", r, err)
	}
}

// with --skip-existing, a chapter whose number is in the folder under another
// name isn't downloaded, and a number listed twice is downloaded once
func TestDownloadSkipsNumbersInTheFolder(t *testing.T) {
	trashIn(t)
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 1, title: "New title"},
		fakeChapter{number: 2, pages: 1, group: "Alpha"},
		fakeChapter{number: 2, pages: 1, group: "Beta"},
	)
	series, dir := newSeries(t, site, false)
	site.Settings.SkipExisting = true
	writeAt(t, dir, "Test Series 1 - Chapter 1 Old title [Sherlyn].cbz", "old", time.Hour)
	rec := &recorder{}

	if _, err := series.Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}

	// the chapters start as the loop reaches them, so only the skips are in order
	var exists []string
	for _, e := range rec.events {
		if strings.HasPrefix(e, "exists ") {
			exists = append(exists, e)
		}
	}
	want := "exists 1 Test Series 1 - Chapter 1 Old title [Sherlyn].cbz|exists 2 Test Series 2 - Chapter 2 [Alpha].cbz"
	if got := strings.Join(exists, "|"); got != want {
		t.Errorf("skips:\n got %q\nwant %q", got, want)
	}
	if n := rec.count("packed 2"); n != 1 {
		t.Errorf("chapter 2 packed %d times, want once (events %q)", n, rec.events)
	}
	if n := site.served[1]; n != 0 {
		t.Errorf("chapter 1 fetched %d times, want none", n)
	}
	if got, want := names(t, dir), "Test Series 1 - Chapter 1 Old title [Sherlyn].cbz|Test Series 2 - Chapter 2 [Alpha].cbz"; got != want {
		t.Errorf("folder:\n got %q\nwant %q", got, want)
	}
}

// without --skip-existing nothing is correlated or removed
func TestDownloadWithoutSkipExistingKeepsDuplicates(t *testing.T) {
	trashIn(t)
	site := newFakeSite(t)
	series, dir := newSeries(t, site, false)
	writeAt(t, dir, "Test Series 1 - A.cbz", "a", 2*time.Hour)
	writeAt(t, dir, "Test Series 1 - B.cbz", "a", time.Hour)
	if _, err := series.Download(context.Background(), nil, nil, "", &recorder{}); err != nil {
		t.Fatal(err)
	}
	if got := names(t, dir); got != "Test Series 1 - A.cbz|Test Series 1 - B.cbz" {
		t.Errorf("folder: %q", got)
	}
}
