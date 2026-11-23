// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/trash"
)

// noBin makes every folder sit on a drive with no Recycle Bin for the test
func noBin(t *testing.T) {
	t.Helper()
	was := trash.HasRecycleBin
	trash.HasRecycleBin = func(string) bool { return false }
	t.Cleanup(func() { trash.HasRecycleBin = was })
}

// pendingAsk waits for the window's question, nil when none comes
func pendingAsk(t *testing.T, c *Controller) *PermanentDTO {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if p := c.PendingPermanent(); p != nil {
			return p
		}
	}
	return nil
}

// the dedupe's question waits for the window's answer: a wrong id is
// refused, the answer comes back, and the question goes
func TestConfirmPermanentWaitsForTheAnswer(t *testing.T) {
	for _, yes := range []bool{false, true} {
		c := NewController(&grabber.Settings{}, nil)
		g, e := attach(c)
		got := make(chan bool)
		go func() { got <- e.ConfirmPermanent("J:\\Manga\\X", 2, 6_400_000) }()
		p := pendingAsk(t, c)
		if p == nil {
			t.Fatal("no question")
		}
		if p.Group != g.id || p.Dir != "J:\\Manga\\X" || p.Files != 2 || p.Bytes != 6_400_000 {
			t.Errorf("question %+v", p)
		}
		if err := c.AnswerPermanent(p.ID+1, true); err == nil {
			t.Error("an answer to another question was taken")
		}
		if err := c.AnswerPermanent(p.ID, yes); err != nil {
			t.Fatal(err)
		}
		select {
		case answer := <-got:
			if answer != yes {
				t.Errorf("answered %v, got %v", yes, answer)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("the answer never came")
		}
		if c.PendingPermanent() != nil {
			t.Error("the question is still there")
		}
		if err := c.AnswerPermanent(p.ID, yes); err == nil {
			t.Error("a second answer was taken")
		}
	}
}

// cancelling the group answers no
func TestConfirmPermanentCancelled(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	g, e := attach(c)
	stop := make(chan struct{})
	g.stop = stop
	got := make(chan bool)
	go func() { got <- e.ConfirmPermanent("J:\\", 1, 1) }()
	if pendingAsk(t, c) == nil {
		t.Fatal("no question")
	}
	close(stop)
	select {
	case answer := <-got:
		if answer {
			t.Error("a cancelled group deleted for good")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelling didn't answer")
	}
	if c.PendingPermanent() != nil {
		t.Error("the question is still there")
	}
}

// the header and log tell kept and deleted-for-good duplicates apart from
// moved ones, and a kept one isn't a failure
func TestCorrelatedPermanentAndKept(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	_, e := attach(c)
	e.Correlated(&core.Correlation{Dir: "J:\\X", Removed: []core.Duplicate{{Name: "a"}}, RemovedBytes: 5, Permanent: true})
	e.Correlated(&core.Correlation{Dir: "J:\\X", Failed: []core.Duplicate{{Name: "b", Err: trash.ErrNoRecycleBin}, {Name: "c", Err: errors.New("busy")}}})
	cl := lastGroup(c).Cleanup
	if cl == nil || cl.Removed != 1 || !cl.Permanent || cl.Kept != 1 || cl.Failed != 1 {
		t.Fatalf("cleanup %+v", cl)
	}
	keys := map[string]bool{}
	for _, l := range c.Log() {
		if l.Msg != nil {
			keys[l.Msg.Key] = true
		}
	}
	if !keys["log.removedForGood"] || !keys["log.keptNoBin"] || keys["log.removed"] {
		t.Errorf("log keys %v", keys)
	}
}

// Delete series on a drive with no Recycle Bin: the window is told, a delete
// it didn't confirm takes nothing, and a confirmed one deletes for good
func TestDeleteSeriesWithoutRecycleBin(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	c, _, _ := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Configure(func(s *grabber.Settings) { s.SeriesFolders = true }); err != nil {
		t.Fatal(err)
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
	noBin(t)

	info, err := c.DeleteInfo()
	if err != nil {
		t.Fatal(err)
	}
	if !info.Permanent {
		t.Fatalf("info = %+v, want permanent", info)
	}
	err = c.DeleteSeries(false, nil)
	var ue *uiError
	if !errors.As(err, &ue) || ue.Message.Key != "errors.deletePermanentNotConfirmed" {
		t.Fatalf("unconfirmed delete: %v", err)
	}
	if got, _ := os.ReadDir(info.Dir); len(got) != 2 {
		t.Fatalf("unconfirmed delete took files: %v", got)
	}
	if err := c.DeleteSeries(true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Dir); !os.IsNotExist(err) {
		t.Errorf("the series folder is still there: %v", err)
	}
	if got, _ := os.ReadDir(filepath.Join(data, "Trash", "files")); len(got) != 0 {
		t.Errorf("deleted for good, yet the trash has %d entries", len(got))
	}
}
