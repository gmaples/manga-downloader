// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"sync"
)

// forkState is the state the fork adds to the Controller: download groups,
// history, the browser hold, VPN hints, the chapter marks' folder look and
// the open "delete for good?" question. Controller embeds it, so its fields
// read as the Controller's own.
type forkState struct {
	// hold is browser.Hold, replaced in tests
	hold func() (release func())

	// loadRelease releases the browser hold of the loaded series, kept from
	// its load so its download reuses the browser (and window) the load
	// opened. Start, the next load and Clear release it; nil when none is kept.
	// Guarded by mu.
	loadRelease func()

	// vpn holds, by site key, why a VPN might help with the site (see
	// http.VPNHint), from its last failure that was a sign of a block. A load
	// or a chapter that works clears it.
	vpn map[string]string

	// groups are the downloads started (one per Start), running or finished,
	// in the order they started; Clear drops the finished ones
	groups []*group
	// lastID numbers the groups, and lastEnd orders their endings
	lastID  int
	lastEnd uint64
	// started holds, by URL, the chapters (by chapKey) selected when Download
	// was last pressed for that URL, so the same selection isn't started again
	started map[string]map[string]bool
	// flightSeq orders the chapters in flight across groups (see flight)
	flightSeq uint64

	// logFile is where every log line is appended too, nil for nowhere (see
	// SetLogFile)
	logFile *os.File

	// history is history.txt's series, newest first, and historyPath where
	// it's saved, "" for nowhere (see history.go)
	history     []HistoryEntry
	historyPath string
	// histBase is the history as history.txt had it at the load or the
	// last save, which a save tells this copy's changes from by (see
	// mergeHistory)
	histBase []HistoryEntry
	// histSave serialises the saves of history; histKick asks the saver
	// goroutine for one, nil when there's no saver
	histSave sync.Mutex
	histKick chan struct{}
	// resumeURL is the URL whose load selects resumeTodo, the to-do chapters
	// of a restored group by label (see Resume)
	resumeURL  string
	resumeTodo map[string]bool
	// resumeKeys are chapters by chapKey that Restart readied with
	// resumeURL, and resumeStart has the load that selects them start them
	// too (startPending, until FinishLoad starts it); see Restart
	resumeKeys map[string]bool
	// resumeIDs are chapters by historyID that Resume or Restart readied
	// with resumeURL, each with its label, used when the listing no longer
	// has the id
	resumeIDs    map[string]string
	resumeStart  bool
	startPending bool
	// adult is set when the site rates the loaded series 18+, and
	// adultTitles holds the titles of the adult series seen this session,
	// which the log masks (see maskLineLocked)
	adult       bool
	adultTitles map[string]bool
	// onDisk is which chapters of the loaded list have their file in the
	// folder, and onDiskKey what it was worked out for (see diskMarksLocked)
	onDisk     []bool
	onDiskName []string
	onDiskKey  string
	// permanent is the open "delete for good?" question (see permanent.go)
	permanent permanentState
}
