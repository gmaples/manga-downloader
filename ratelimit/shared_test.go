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

// two books of one folder stand for two copies of the program
func twoCopies(t *testing.T, file string) (a, b *Book, dir string) {
	t.Helper()
	dir = t.TempDir()
	if file != "" {
		os.WriteFile(filepath.Join(dir, FileName), []byte(file), 0o644)
	}
	return Open(dir, nil, nil), Open(dir, nil, nil), dir
}

func wait(t *testing.T, s *Site) {
	t.Helper()
	if _, err := s.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Done(0, false)
}

// the copies' requests together keep the site's gap
func TestCopiesShareTheGap(t *testing.T) {
	a, b, _ := twoCopies(t, "x.org: 50ms\n")
	sa, sb := a.Site("x.org"), b.Site("x.org")
	start := time.Now()
	wait(t, sa)
	wait(t, sb)
	wait(t, sa)
	wait(t, sb)
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Errorf("4 requests from two copies took %v, want at least 3 gaps of 50ms", elapsed)
	}
}

// a 429 one copy gets pauses the other, which had no time for the site
func TestCopiesShareThePause(t *testing.T) {
	shrink(t, 10*time.Millisecond, time.Minute, 150*time.Millisecond, time.Minute)
	a, b, _ := twoCopies(t, "")
	sa, sb := a.Site("x.org"), b.Site("x.org")
	wait(t, sa)
	sa.Hit(sa.gen, 0)
	start := time.Now()
	wait(t, sb)
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Errorf("the other copy went after %v, within the 150ms pause", elapsed)
	}
	// and then keeps the gap the 429 set, though it has none of its own
	start = time.Now()
	wait(t, sb)
	if elapsed := time.Since(start); elapsed < 8*time.Millisecond {
		t.Errorf("the other copy's next request went after %v, want the 10ms gap", elapsed)
	}
}

// a site with no gap and no pause anywhere costs no file
func TestNoSharedPaceForAnUnpacedSite(t *testing.T) {
	a, _, dir := twoCopies(t, "")
	wait(t, a.Site("x.org"))
	if _, err := os.Stat(filepath.Join(dir, PaceFileName)); !os.IsNotExist(err) {
		t.Errorf("%s: %v, want none", PaceFileName, err)
	}
}

// the file is read and written as plain key: value lines, and a lock left by
// a copy that stopped is taken over
func TestPaceFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, PaceFileName)
	next := time.Date(2026, 11, 21, 18, 0, 0, 5, time.UTC)
	m := map[string]paceEntry{
		"x.org":   {next: next, gap: 1500 * time.Millisecond},
		"old.org": {next: time.Now().Add(-time.Hour)},
	}
	if err := writePace(path, m); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "x.org next: 2026-11-21T18:00:00.000000005Z\nx.org gap: 1.5s\n") || strings.Contains(string(data), "old.org") {
		t.Errorf("file is %q", data)
	}
	got := readPace(path)["x.org"]
	if !got.next.Equal(next) || got.gap != 1500*time.Millisecond {
		t.Errorf("read back %+v", got)
	}

	// a lock file a stopped copy left behind isn't held, so it's taken
	lock := filepath.Join(dir, PaceLockName)
	os.WriteFile(lock, nil, 0o644)
	unlock, err := lockPace(dir)
	if err != nil {
		t.Fatalf("left-behind lock file: %v", err)
	}
	unlock()
}

// another copy's 429 restarts this copy's calm
func TestAnotherCopysPauseRestartsTheCalm(t *testing.T) {
	shrink(t, 10*time.Millisecond, time.Minute, 30*time.Millisecond, time.Minute)
	a, b, _ := twoCopies(t, "x.org: 20ms\n")
	sa, sb := a.Site("x.org"), b.Site("x.org")
	wait(t, sb)
	sb.mu.Lock()
	sb.calm = time.Now().Add(-time.Hour)
	sb.mu.Unlock()
	sa.Hit(sa.gen, 0)
	wait(t, sb)
	sb.mu.Lock()
	defer sb.mu.Unlock()
	if time.Since(sb.calm) > time.Second {
		t.Errorf("calm still from %v ago", time.Since(sb.calm))
	}
	if sb.gap != 20*time.Millisecond {
		t.Errorf("gap %v, want the 20ms it had (not lowered)", sb.gap)
	}
}
