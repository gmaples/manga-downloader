// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// selectOnly selects chapter indices of the loaded list and nothing else
func selectOnly(t *testing.T, c *Controller, indices ...int) {
	t.Helper()
	if err := c.SetAll(false); err != nil {
		t.Fatal(err)
	}
	if err := c.SetSelectedMany(indices, true); err != nil {
		t.Fatal(err)
	}
}

// wantMode checks what Download would do
func wantMode(t *testing.T, c *Controller, what string, adding bool, fresh int, unchanged bool) {
	t.Helper()
	addTo, n, same := c.DownloadMode()
	if (addTo != 0) != adding || n != fresh || same != unchanged {
		t.Errorf("%s: DownloadMode = %d, %d, %v; want adding %v, %d fresh, unchanged %v", what, addTo, n, same, adding, fresh, unchanged)
	}
}

// While a series downloads, Download adds the chapters it doesn't have yet to
// its run, which ends with one summary for all of them. The same selection
// can't be started again, during the run or after it.
func TestAddToARunningSeries(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, dir := newTwoSeries(t, gateA, nil)
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 0)
	wantMode(t, c, "before the run", false, 0, false)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered

	wantMode(t, c, "same selection while running", true, 0, false)
	if m := messageOf(c.Start()); m == nil || m.Key != "errors.nothingNew" {
		t.Errorf("Start with nothing new = %v, want errors.nothingNew", m)
	}

	selectOnly(t, c, 0, 1)
	wantMode(t, c, "one more chapter while running", true, 1, false)
	if err := c.Start(); err != nil {
		t.Fatalf("Start adding chapter 2 = %v", err)
	}
	if n := len(c.Groups()); n != 1 {
		t.Fatalf("%d groups, want the chapter added to the 1 running", n)
	}
	if r := runOf(c); r.Chapters != 2 || r.Rows != 2 {
		t.Errorf("run = %+v, want 2 chapters in 2 rows", r)
	}
	if !logHas(c, "Added 1 chapter of Series A to its download") {
		t.Errorf("log = %+v, want the chapter added", c.Log())
	}
	wantMode(t, c, "after adding", true, 0, false)

	close(gateA)
	s := waitGroup(t, c, "Series A")
	if s.Saved != 2 || s.Cancelled || s.Err != nil || len(s.Failures) != 0 {
		t.Errorf("summary = %+v, want both chapters saved", s)
	}
	if files := cbzNamed(t, dir, "Series A"); len(files) != 2 {
		t.Errorf("files = %v, want both chapters", files)
	}
	if jobs := jobsOf(c); len(jobs) != 2 {
		t.Errorf("rows = %+v, want one per chapter", jobs)
	}

	wantMode(t, c, "same selection after the run", false, 0, true)
	selectOnly(t, c, 1)
	wantMode(t, c, "changed selection after the run", false, 0, false)
}

// A reload of the series' URL lists new chapter objects, which are matched to
// the run's by number, title, language and groups
func TestAddAfterAReload(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, dir := newTwoSeries(t, gateA, nil)
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 0)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered

	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAll(true); err != nil {
		t.Fatal(err)
	}
	wantMode(t, c, "after the reload", true, 1, false)
	if err := c.Start(); err != nil {
		t.Fatalf("Start after the reload = %v", err)
	}
	close(gateA)
	if s := waitGroup(t, c, "Series A"); s.Saved != 2 {
		t.Errorf("summary = %+v, want 2 saved", s)
	}
	if files := cbzNamed(t, dir, "Series A"); len(files) != 2 {
		t.Errorf("files = %v, want each chapter once", files)
	}
}

// Cancelling the group drops the chapters added to it that haven't started
func TestCancelDropsAddedChapters(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, dir := newTwoSeries(t, gateA, nil)
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 0)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered
	selectOnly(t, c, 0, 1)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}

	c.Cancel()
	close(gateA)
	s := waitGroup(t, c, "Series A")
	if !s.Cancelled || s.Saved != 1 {
		t.Errorf("summary = %+v, want cancelled with the chapter in flight saved", s)
	}
	if files := cbzNamed(t, dir, "Series A"); len(files) != 1 {
		t.Errorf("files = %v, want only chapter 1", files)
	}
}

// Added chapters keep the group's settings, and with One file they make a
// file of their own, named by their own range
func TestAddToABundle(t *testing.T) {
	gateA := make(chan struct{})
	c, sites, dir := newTwoSeries(t, gateA, nil)
	c.Configure(func(s *grabber.Settings) { s.Bundle = true })
	if err := c.Load(urlA); err != nil {
		t.Fatal(err)
	}
	selectOnly(t, c, 0)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	<-sites(urlA).entered

	// the window's options no longer reach the group
	c.Configure(func(s *grabber.Settings) { s.Bundle = false })
	selectOnly(t, c, 0, 1)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	if r := runOf(c); !r.Bundle || r.Chapters != 2 || r.Rows != 2 {
		t.Errorf("run = %+v, want a bundle run of 2 chapters in 2 rows", r)
	}
	close(gateA)
	s := waitGroup(t, c, "Series A")
	if s.Saved != 2 || s.Err != nil || !s.Bundled {
		t.Fatalf("summary = %+v, want 2 chapters bundled", s)
	}
	names := strings.Split(s.Bundle, ", ")
	if len(names) != 2 || names[0] == names[1] {
		t.Errorf("bundles = %q, want two files", s.Bundle)
	}
	if files := cbzNamed(t, dir, "Series A"); len(files) != 2 {
		t.Errorf("files = %v, want two bundles", files)
	}
	jobs := jobsOf(c)
	if len(jobs) != 2 || jobs[0].State != JobDone || jobs[1].State != JobDone {
		t.Errorf("rows = %+v, want two bundle rows, both done", jobs)
	}
}
