// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// learnFast shrinks the learning values for a test
func learnFast(t *testing.T, window, hold time.Duration, probeEvery int) {
	t.Helper()
	w, h, p, sp, mp := Window, Hold, ProbeEvery, StartParallel, MaxParallel
	Window, Hold, ProbeEvery = window, hold, probeEvery
	t.Cleanup(func() { Window, Hold, ProbeEvery, StartParallel, MaxParallel = w, h, p, sp, mp })
}

func readParallelFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ParallelFileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseParallelLine(t *testing.T) {
	for _, c := range []struct {
		line string
		key  string
		n    int
		ok   bool
		bad  bool
	}{
		{line: "mangadex.org: 8", key: "mangadex.org", n: 8, ok: true},
		{line: "  WWW.Comix.to :12\r", key: "comix.to", n: 12, ok: true},
		{line: "# a comment"},
		{line: ""},
		{line: "mangadex.org 8", bad: true},
		{line: "mangadex.org: 0", bad: true},
		{line: "mangadex.org: lots", bad: true},
		{line: ": 4", bad: true},
	} {
		key, n, ok, err := parseParallelLine(c.line)
		if (err != nil) != c.bad || key != c.key || n != c.n || ok != c.ok {
			t.Errorf("parseParallelLine(%q) = %q, %d, %v, %v", c.line, key, n, ok, err)
		}
	}
}

// a saved level is where the site starts, without doubling; a site with none
// starts at StartParallel; a bad line is reported and skipped
func TestSavedLevelIsTheStart(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ParallelFileName), []byte("# mine\nx.org: 7\ny.org: many\nbig.org: 900\n"), 0o644)
	warn := &lines{}
	b := Open(dir, warn.add, nil)
	if s := b.Site("x.org"); s.Parallel() != 7 || s.doubling {
		t.Errorf("x.org starts at %d (doubling %v), want the saved 7", s.Parallel(), s.doubling)
	}
	if s := b.Site("new.org"); s.Parallel() != StartParallel || !s.doubling {
		t.Errorf("new.org starts at %d (doubling %v), want %d, doubling", s.Parallel(), s.doubling, StartParallel)
	}
	if n := b.Site("big.org").Parallel(); n != MaxParallel {
		t.Errorf("big.org starts at %d, want the cap %d", n, MaxParallel)
	}
	if len(warn.got) != 1 || !strings.Contains(warn.got[0], "line 3") {
		t.Errorf("warnings = %q, want one for line 3", warn.got)
	}
}

// no more than the level are let out at once; Done lets the next one go
func TestSlotsHoldTheLevel(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ParallelFileName), []byte("x.org: 2\n"), 0o644)
	s := Open(dir, nil, nil).Site("x.org")
	for range 2 {
		if _, err := s.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	third := make(chan struct{})
	go func() {
		s.Wait(context.Background())
		close(third)
	}()
	select {
	case <-third:
		t.Fatal("a third request went out at level 2")
	case <-time.After(30 * time.Millisecond):
	}
	s.Done(10, false)
	select {
	case <-third:
	case <-time.After(time.Second):
		t.Fatal("Done didn't let the queued request go")
	}

	// a cancelled queued request gives up its place
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := s.Wait(ctx); err != context.Canceled {
		t.Errorf("cancelled wait = %v", err)
	}
	if s.waiting != 0 || s.inflight != 2 {
		t.Errorf("waiting %d, in flight %d after the cancel, want 0 and 2", s.waiting, s.inflight)
	}
}

// trouble halves the level and saves it at once; more trouble right after,
// from requests sent before the cut, doesn't cut again; the level never
// rises back to the one that had trouble
func TestTroubleCuts(t *testing.T) {
	learnFast(t, 40*time.Millisecond, 0, 0)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ParallelFileName), []byte("x.org: 8\n"), 0o644)
	info := &lines{}
	s := Open(dir, nil, info.add).Site("x.org")
	for range 3 {
		s.Wait(context.Background())
	}
	s.Done(0, true)
	s.Done(0, true)
	if n := s.Parallel(); n != 4 {
		t.Errorf("level after trouble = %d, want 4", n)
	}
	if got := readParallelFile(t, dir); !strings.Contains(got, "x.org: 4\n") {
		t.Errorf("file =\n%s\nwant x.org: 4", got)
	}
	if len(info.got) != 1 || !strings.Contains(info.got[0], "down from 8 to 4") {
		t.Errorf("reported %q", info.got)
	}
	s.mu.Lock()
	top := s.topLocked()
	s.mu.Unlock()
	if top != 7 {
		t.Errorf("the level may rise to %d, want 7, below the 8 that had trouble", top)
	}
	time.Sleep(50 * time.Millisecond)
	s.Done(0, true)
	if n := s.Parallel(); n != 2 {
		t.Errorf("level after trouble a window later = %d, want 2", n)
	}
}

// a run whose level doesn't move writes no file; the last download sharing a
// book to end saves a level that moved
func TestReleaseSavesWhatMoved(t *testing.T) {
	dir := t.TempDir()
	b, release := Share(dir, nil, nil)
	b.Site("x.org")
	release()
	if _, err := os.Stat(filepath.Join(dir, ParallelFileName)); !os.IsNotExist(err) {
		t.Errorf("a run that learned nothing wrote %s: %v", ParallelFileName, err)
	}

	b, releaseA := Share(dir, nil, nil)
	_, releaseB := Share(dir, nil, nil)
	s := b.Site("x.org")
	s.mu.Lock()
	s.par = 9
	s.mu.Unlock()
	if Parallel("x.org") != 9 {
		t.Errorf("Parallel = %d, want 9", Parallel("x.org"))
	}
	releaseA()
	if _, err := os.Stat(filepath.Join(dir, ParallelFileName)); !os.IsNotExist(err) {
		t.Error("saved while another download still shares the book")
	}
	releaseB()
	if got := readParallelFile(t, dir); !strings.HasPrefix(got, "# ") || !strings.HasSuffix(got, "\nx.org: 9\n") {
		t.Errorf("file =\n%s\nwant the header and x.org: 9", got)
	}
	if Parallel("x.org") != 0 {
		t.Errorf("Parallel = %d once no download runs, want 0", Parallel("x.org"))
	}
}

// judge walks the level through a site whose speed tops out at 8 at once:
// doubling while it pays, back to the best, a try of one more that doesn't
// pay, and back again
func TestJudgeFindsTheBest(t *testing.T) {
	learnFast(t, 0, 0, 1)
	s := Open(t.TempDir(), nil, nil).Site("x.org")
	speed := func(par int) int64 { return int64(min(par, 8)) * 1000 }
	window := func() string {
		s.mu.Lock()
		defer s.mu.Unlock()
		// a busy window of 1s at the current level
		s.winStart = time.Now().Add(-time.Second)
		s.winBytes, s.winDone, s.winBusy = speed(s.par), 10, 10
		s.warm = false // the warm-up window after a change is TestWarmUpWindow's
		return s.judgeLocked(time.Now())
	}
	var levels []int
	var settled string
	for range 7 {
		if msg := window(); msg != "" {
			settled = msg
		}
		levels = append(levels, s.Parallel())
	}
	want := []int{8, 16, 8, 8, 9, 8, 8}
	if !equalInts(levels, want) {
		t.Errorf("levels = %v, want %v", levels, want)
	}
	if !strings.Contains(settled, "settled at 8") {
		t.Errorf("reported %q, want settled at 8", settled)
	}

	// a window without requests queued teaches nothing
	s.mu.Lock()
	before := s.par
	s.winStart = time.Now().Add(-time.Second)
	s.winBytes, s.winDone, s.winBusy = 1, 10, 2
	s.warm = false
	s.judgeLocked(time.Now())
	after := s.par
	s.mu.Unlock()
	if after != before {
		t.Errorf("an idle window moved the level from %d to %d", before, after)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// the window right after a change of level doesn't count
func TestWarmUpWindow(t *testing.T) {
	learnFast(t, 0, 0, 1)
	s := Open(t.TempDir(), nil, nil).Site("x.org")
	judge := func(bytes int64) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.winStart = time.Now().Add(-time.Second)
		s.winBytes, s.winDone, s.winBusy = bytes, 10, 10
		s.judgeLocked(time.Now())
	}
	judge(4000) // 4 doubles to 8
	if n := s.Parallel(); n != 8 {
		t.Fatalf("level = %d, want 8", n)
	}
	judge(1) // warm-up: a slow window teaches nothing
	if n := s.Parallel(); n != 8 {
		t.Errorf("the warm-up window moved the level to %d", n)
	}
	judge(8000) // twice as fast: doubles on
	if n := s.Parallel(); n != 16 {
		t.Errorf("level = %d, want 16", n)
	}
}
