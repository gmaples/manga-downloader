package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

// slowMkdir has Start's folder take a while to make, and runs during first
func slowMkdir(t *testing.T, during func()) (making chan struct{}) {
	t.Helper()
	making = make(chan struct{}, 1)
	saved := mkdirAll
	mkdirAll = func(path string, perm os.FileMode) error {
		making <- struct{}{}
		if during != nil {
			during()
		}
		time.Sleep(200 * time.Millisecond)
		return saved(path, perm)
	}
	t.Cleanup(func() { mkdirAll = saved })
	return making
}

func readyToStart(t *testing.T, c *Controller) {
	t.Helper()
	if err := c.Load(testURL); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.SeriesFolders = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1-2"); err != nil {
		t.Fatal(err)
	}
}

// Start makes the series folder with the controller free: a slow card
// doesn't stall the window
func TestStartMakesTheFolderWithTheControllerFree(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	readyToStart(t, c)
	making := slowMkdir(t, nil)

	started := make(chan error)
	go func() { started <- c.Start() }()
	<-making
	start := time.Now()
	c.Title()
	if d := time.Since(start); d > 20*time.Millisecond {
		t.Errorf("Title took %v while the folder was being made, want under 20ms", d)
	}
	if err := <-started; err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFinished(t, c)
}

// a load while the folder is being made wins: that Start starts nothing
func TestStartGivesWayToALoadWhileTheFolderIsMade(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	readyToStart(t, c)
	slowMkdir(t, func() {
		if err := c.Load(testURL); err != nil {
			t.Error(err)
		}
	})

	if err := c.Start(); !errors.Is(err, errBusy) {
		t.Fatalf("Start = %v, want errBusy", err)
	}
	if c.Running() {
		t.Error("a run started for the old list")
	}
}
