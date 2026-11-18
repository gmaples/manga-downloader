// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"sync"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// holdCounter stands in for browser.Hold: it counts the holds not yet
// released, and how many were ever taken
type holdCounter struct {
	mu    sync.Mutex
	open  int
	taken int
}

func (h *holdCounter) hold() func() {
	h.mu.Lock()
	h.open++
	h.taken++
	h.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			h.mu.Lock()
			h.open--
			h.mu.Unlock()
		})
	}
}

func (h *holdCounter) counts() (open, taken int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.open, h.taken
}

func newHoldController(t *testing.T) (*Controller, *holdCounter) {
	t.Helper()
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	h := &holdCounter{}
	c.hold = h.hold
	c.closeBrowser = func() {}
	return c, h
}

// A loaded series keeps the browser held, and its download takes the hold
// over: the browser never drops to no holds between the load and the run
// (which would close it and open a new window), and none is left at the end.
func TestLoadKeepsTheBrowserForTheDownload(t *testing.T) {
	c, h := newHoldController(t)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if open, _ := h.counts(); open != 1 {
		t.Fatalf("holds after the load = %d, want 1 (kept for the download)", open)
	}

	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	if open, taken := h.counts(); open != 0 || taken != 2 {
		t.Errorf("after the run: %d holds open, %d taken; want 0 open, 2 taken (load, run)", open, taken)
	}
}

func TestClearReleasesTheLoadHold(t *testing.T) {
	c, h := newHoldController(t)
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	c.closing.Wait()
	if open, _ := h.counts(); open != 0 {
		t.Errorf("holds after Clear = %d, want 0", open)
	}
}

// Loading another series swaps the kept hold for its own; a failed load keeps none.
func TestNextLoadReplacesTheLoadHold(t *testing.T) {
	c, h := newHoldController(t)
	for i := 0; i < 2; i++ {
		if err := c.Load("https://example.com/manga/test"); err != nil {
			t.Fatal(err)
		}
		if open, _ := h.counts(); open != 1 {
			t.Fatalf("holds after load %d = %d, want 1", i+1, open)
		}
	}

	c.identify = func(string, *grabber.Settings) (grabber.Site, []error) { return nil, nil }
	if err := c.Load("https://example.com/manga/none"); err == nil {
		t.Fatal("a load no site recognises succeeded")
	}
	if open, _ := h.counts(); open != 0 {
		t.Errorf("holds after a failed load = %d, want 0", open)
	}
}
