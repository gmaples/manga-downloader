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

// a listed site's gap is divided by Growth after each CalmFor without a 429,
// and the site stops being listed once the gap would go below StartGap
func TestCalmDownLowersThenClears(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("mangalib.org: 2.25s\n"), 0o644)
	s := Open(dir, nil, nil).Site("mangalib.org")
	t0 := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()
	if got, cleared := s.calmDownLocked(t0, true, time.Time{}); got != 0 || cleared {
		t.Fatalf("first request: %v %v, want nothing (the calm starts)", got, cleared)
	}
	if got, cleared := s.calmDownLocked(t0.Add(CalmFor - time.Second), true, time.Time{}); got != 0 || cleared {
		t.Fatalf("before CalmFor: %v %v, want nothing", got, cleared)
	}
	if got, _ := s.calmDownLocked(t0.Add(CalmFor), true, time.Time{}); got != 1500*time.Millisecond {
		t.Fatalf("after CalmFor: %v, want 1.5s", got)
	}
	if got, _ := s.calmDownLocked(t0.Add(CalmFor + time.Minute), true, time.Time{}); got != 0 {
		t.Fatalf("a minute later: %v, want nothing (the calm restarted)", got)
	}
	if got, _ := s.calmDownLocked(t0.Add(2 * CalmFor), true, time.Time{}); got != time.Second {
		t.Fatalf("after 2 CalmFor: %v, want 1s", got)
	}
	if got, cleared := s.calmDownLocked(t0.Add(3 * CalmFor), true, time.Time{}); got != 0 || !cleared {
		t.Fatalf("after 3 CalmFor: %v %v, want cleared", got, cleared)
	}
	if s.listed || s.gap != 0 {
		t.Errorf("listed %v gap %v, want unlisted with no gap", s.listed, s.gap)
	}
	if got, cleared := s.calmDownLocked(t0.Add(9 * CalmFor), true, time.Time{}); got != 0 || cleared {
		t.Errorf("unlisted site: %v %v, want nothing", got, cleared)
	}
}

// a 429 restarts the calm
func TestHitRestartsTheCalm(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("x.org: 4s\n"), 0o644)
	s := Open(dir, nil, nil).Site("x.org")
	s.mu.Lock()
	s.calmDownLocked(time.Now().Add(-2 * CalmFor), true, time.Time{})
	s.mu.Unlock()
	s.Hit(0, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	if got, cleared := s.calmDownLocked(time.Now(), true, time.Time{}); got != 0 || cleared {
		t.Errorf("right after a 429: %v %v, want nothing", got, cleared)
	}
}

// through Wait, the lowered gap is saved and the cleared line is taken out of
// the file, every other line kept
func TestWaitCalmsDownTheFile(t *testing.T) {
	calm := CalmFor
	CalmFor = 30 * time.Millisecond
	t.Cleanup(func() { CalmFor = calm })
	shrink(t, 10*time.Millisecond, time.Minute, time.Second, time.Minute)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("# mine\nx.org: 20ms\nother.org: 5s\n"), 0o644)
	var info lines
	s := Open(dir, nil, info.add).Site("x.org")
	wait := func() {
		t.Helper()
		if _, err := s.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		s.Done(0, false)
	}
	wait()
	time.Sleep(CalmFor)
	wait()
	if got := read(t, dir); !strings.Contains(got, "x.org: 13ms\n") {
		t.Fatalf("after one calm the file is %q, want x.org: 13ms", got)
	}
	time.Sleep(CalmFor)
	wait()
	if got, want := read(t, dir), "# mine\nother.org: 5s\n"; got != want {
		t.Errorf("after two calms the file is %q, want %q", got, want)
	}
	if s.Listed() {
		t.Error("the site is still listed")
	}
	if len(info.got) != 2 {
		t.Errorf("reported %q, want two lines", info.got)
	}
}
