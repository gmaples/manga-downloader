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

// shrinkClaims sets the claim timings for a test, restoring them afterwards
func shrinkClaims(t *testing.T) {
	t.Helper()
	b, s, p := ClaimBeat, ClaimStale, ClaimPoll
	ClaimBeat, ClaimStale, ClaimPoll = 20*time.Millisecond, 200*time.Millisecond, 5*time.Millisecond
	t.Cleanup(func() { ClaimBeat, ClaimStale, ClaimPoll = b, s, p })
}

// chapterFile downloads the one chapter of site once into a scratch folder to
// learn its file's name, the one a claim is named after
func chapterFile(t *testing.T, site *fakeSite) string {
	t.Helper()
	series, dir := newSeries(t, site, false)
	if _, err := series.Download(context.Background(), listed(site), nil, "", &recorder{}); err != nil {
		t.Fatal(err)
	}
	files := cbzFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("wrote %q, want one file", files)
	}
	return filepath.Base(files[0])
}

func TestTryClaim(t *testing.T) {
	shrinkClaims(t)
	path := filepath.Join(t.TempDir(), "sub", "a.cbz"+ClaimSuffix)
	c, ok, err := tryClaim(path)
	if err != nil || !ok {
		t.Fatalf("first claim: %v %v", ok, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "process: ") {
		t.Errorf("claim holds %q, want key: value lines", data)
	}
	if _, ok, err := tryClaim(path); ok || err != nil {
		t.Fatalf("second claim: %v %v, want refused", ok, err)
	}
	// the beat keeps it live past ClaimStale
	time.Sleep(2 * ClaimStale)
	if _, ok, _ := tryClaim(path); ok {
		t.Fatal("a beating claim was taken over")
	}
	c.release()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("released claim still there: %v", err)
	}
}

// a claim untouched for ClaimStale was left by a copy that stopped: taken over
func TestTryClaimTakesOverAStaleOne(t *testing.T) {
	shrinkClaims(t)
	path := filepath.Join(t.TempDir(), "a.cbz"+ClaimSuffix)
	os.WriteFile(path, []byte("process: 1\n"), 0o644)
	old := time.Now().Add(-2 * ClaimStale)
	os.Chtimes(path, old, old)
	c, ok, err := tryClaim(path)
	if err != nil || !ok {
		t.Fatalf("stale claim: %v %v, want taken over", ok, err)
	}
	c.release()
}

// while another copy holds the claim, the chapter waits; when that copy saved
// the file, it's skipped as existing, with nothing downloaded twice
func TestDownloadWaitsForAnotherCopy(t *testing.T) {
	shrinkClaims(t)
	site := newFakeSite(t, fakeChapter{number: 1, pages: 2})
	name := chapterFile(t, site)
	series, dir := newSeries(t, site, false)
	series.settings.SkipExisting = true

	claimPath := filepath.Join(dir, name) + ClaimSuffix
	other, ok, err := tryClaim(claimPath)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	rec := &recorder{}
	done := make(chan error)
	go func() {
		_, err := series.Download(context.Background(), listed(site), nil, "", rec)
		done <- err
	}()
	waitFor(t, rec, "waiting 1 "+name)
	// the other copy saves the file, then lets go of the claim
	os.WriteFile(filepath.Join(dir, name), []byte("saved by the other copy"), 0o644)
	other.release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.events, "|"); got != "waiting 1 "+name+"|exists 1 "+name {
		t.Errorf("events %q, want waiting then exists", got)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, name)); string(data) != "saved by the other copy" {
		t.Error("the other copy's file was replaced")
	}
	if files := cbzFiles(t, dir); len(files) != 1 {
		t.Errorf("files %q, want only the other copy's", files)
	}
}

// when the other copy lets go without saving (it failed), the chapter is
// downloaded, and its claim is gone afterwards
func TestDownloadAfterAnotherCopyFailed(t *testing.T) {
	shrinkClaims(t)
	site := newFakeSite(t, fakeChapter{number: 1, pages: 2})
	name := chapterFile(t, site)
	series, dir := newSeries(t, site, false)
	series.settings.SkipExisting = true

	claimPath := filepath.Join(dir, name) + ClaimSuffix
	other, _, _ := tryClaim(claimPath)
	rec := &recorder{}
	done := make(chan error)
	go func() {
		_, err := series.Download(context.Background(), listed(site), nil, "", rec)
		done <- err
	}()
	waitFor(t, rec, "waiting 1 "+name)
	other.release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(rec.events, "|"); !strings.HasSuffix(got, "packed 1") {
		t.Errorf("events %q, want the chapter packed", got)
	}
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Errorf("chapter file: %v", err)
	}
	if _, err := os.Stat(claimPath); !os.IsNotExist(err) {
		t.Errorf("claim left behind: %v", err)
	}
}

// a cancelled wait fails the chapter with the context's error
func TestDownloadWaitCancelled(t *testing.T) {
	shrinkClaims(t)
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	name := chapterFile(t, site)
	series, dir := newSeries(t, site, false)
	other, _, _ := tryClaim(filepath.Join(dir, name) + ClaimSuffix)
	defer other.release()
	ctx, cancel := context.WithCancel(context.Background())
	rec := &recorder{}
	done := make(chan error)
	go func() {
		_, err := series.Download(ctx, listed(site), nil, "", rec)
		done <- err
	}()
	waitFor(t, rec, "waiting 1 "+name)
	cancel()
	<-done
	if files := cbzFiles(t, dir); len(files) != 0 {
		t.Errorf("files %q, want none", files)
	}
}

// waitFor waits until rec has recorded event
func waitFor(t *testing.T, rec *recorder, event string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec.mu.Lock()
		got := strings.Join(rec.events, "|")
		rec.mu.Unlock()
		if strings.Contains(got, event) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %q event", event)
}
