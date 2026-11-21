// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func entry(url string, minute int, labels ...string) HistoryEntry {
	e := HistoryEntry{Title: url, URL: url, When: time.Date(2026, 11, 21, 10, minute, 0, 0, time.Local)}
	for _, l := range labels {
		e.Chapters = append(e.Chapters, HistoryChapter{Label: l})
	}
	return e
}

func urls(entries []HistoryEntry) string {
	var out []string
	for _, e := range entries {
		out = append(out, e.URL+"("+strings.Join(labels(e), ",")+")")
	}
	return strings.Join(out, " ")
}

func labels(e HistoryEntry) []string {
	var out []string
	for _, ch := range e.Chapters {
		l := ch.Label
		if ch.Done {
			l += "+"
		}
		out = append(out, l)
	}
	return out
}

func TestMergeHistory(t *testing.T) {
	base := []HistoryEntry{entry("a", 1, "1"), entry("b", 2, "1"), entry("c", 3, "1"), entry("d", 4, "1")}
	// this copy: a changed, b as it was, c dropped, e added
	ours := []HistoryEntry{entry("e", 9, "1"), entry("a", 8, "1", "2"), entry("b", 2, "1"), entry("d", 4, "1")}
	// the other copy: b changed, d dropped, f added; a and c as they were
	disk := []HistoryEntry{entry("f", 7, "1"), entry("b", 6, "1", "9"), entry("a", 1, "1"), entry("c", 3, "1")}

	got := urls(mergeHistory(base, ours, disk))
	want := "e(1) a(1,2) f(1) b(1,9)"
	if got != want {
		t.Errorf("merged %s, want %s", got, want)
	}
}

// a series this copy dropped stays dropped, unless the other copy changed it since
func TestMergeHistoryDroppedButChangedElsewhere(t *testing.T) {
	base := []HistoryEntry{entry("a", 1, "1")}
	disk := []HistoryEntry{entry("a", 5, "1", "2")}
	if got := urls(mergeHistory(base, nil, disk)); got != "a(1,2)" {
		t.Errorf("merged %s, want the other copy's a", got)
	}
}

// two copies of the app saving one history.txt keep each other's series and chapters
func TestTwoCopiesShareHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.txt")
	first := NewController(defaultSettings(), func() {})
	first.SetHistory(path, nil)
	second := NewController(defaultSettings(), func() {})
	second.SetHistory(path, nil)

	first.mu.Lock()
	first.touchLocked("https://one.example/a", "A", false).addChapters([]HistoryChapter{{Label: "A 1", Done: true}})
	first.mu.Unlock()
	first.saveHistoryNow()

	second.mu.Lock()
	second.touchLocked("https://two.example/b", "B", false).addChapters([]HistoryChapter{{Label: "B 1"}})
	second.mu.Unlock()
	second.saveHistoryNow()

	// the first copy goes on with its series, then saves again
	first.mu.Lock()
	first.history[0].Chapters = append(first.history[0].Chapters, HistoryChapter{Label: "A 2"})
	first.mu.Unlock()
	first.saveHistoryNow()

	got := urls(loadHistory(path))
	if !strings.Contains(got, "https://one.example/a(A 1+,A 2)") || !strings.Contains(got, "https://two.example/b(B 1)") {
		t.Errorf("history.txt has %s, want both copies' series", got)
	}
	// the first copy now knows the second's series too
	if got := urls(first.History()); !strings.Contains(got, "two.example") {
		t.Errorf("first copy's history %s, want the second's series", got)
	}
}
