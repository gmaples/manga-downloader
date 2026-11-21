// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestHistoryRoundTrip(t *testing.T) {
	when := time.Date(2026, 11, 21, 5, 40, 0, 0, time.Local)
	in := []HistoryEntry{
		{Title: "Adult One", URL: "https://zenko.online/titles/2779", Adult: true, When: when, Open: true,
			Chapters: []HistoryChapter{{Label: "Chapter 1", Done: true}, {Label: "Chapter 2 [uk]"}}},
		{Title: "Plain", URL: "https://example.com/manga/plain", When: when},
	}
	text := string(formatHistory(in))
	for _, want := range []string{
		"title: Adult One\nurl: https://zenko.online/titles/2779\nadult: yes\nwhen: 2026-11-21 05:40\nopen: yes\ndone: Chapter 1\ntodo: Chapter 2 [uk]\n",
		"\ntitle: Plain\nurl: https://example.com/manga/plain\nadult: no\nwhen: 2026-11-21 05:40\nopen: no\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("history.txt lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "{") {
		t.Errorf("history.txt isn't plain key: value lines:\n%s", text)
	}
	out := parseHistory(strings.NewReader(text))
	if !reflect.DeepEqual(out, in) {
		t.Errorf("parsed back\n%+v\nwant\n%+v", out, in)
	}
}

// a file Greg edited by hand: no blank line between two series, a comment,
// an unknown key, other spellings of yes, and a series without a url
func TestParseHistoryTakesHandEdits(t *testing.T) {
	text := `# notes
title: A
url: https://a.example/1
Open: Yes
colour: blue
title: B
url: https://b.example/2
adult: true
done: Chapter 3

title: no url
todo: Chapter 1
`
	got := parseHistory(strings.NewReader(text))
	want := []HistoryEntry{
		{Title: "A", URL: "https://a.example/1", Open: true},
		{Title: "B", URL: "https://b.example/2", Adult: true, Chapters: []HistoryChapter{{Label: "Chapter 3", Done: true}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestHistoryKeepsTheLast100(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	for i := range 105 {
		c.touchLocked("https://example.com/"+string(rune('a'+i%26))+strings.Repeat("x", i), "t", false)
	}
	h := c.History()
	if len(h) != historyMax || h[0].URL != "https://example.com/"+string(rune('a'+104%26))+strings.Repeat("x", 104) {
		t.Fatalf("%d entries, first %q", len(h), h[0].URL)
	}
	// touching an entry moves it to the front instead of adding another
	c.touchLocked(h[50].URL, "t", false)
	if h2 := c.History(); len(h2) != historyMax || h2[0].URL != h[50].URL {
		t.Errorf("touch didn't move the entry to the front")
	}
}

func TestHistoryRecordsDoneAndTodo(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, fakeChapter{number: 1, pages: 2}, fakeChapter{number: 2, pages: 2, fail: map[int]bool{1: true}}, fakeChapter{number: 3, pages: 2})
	path := filepath.Join(t.TempDir(), "history.txt")
	c.SetHistory(path, nil)
	url := "https://example.com/manga/test"
	if err := c.Load(url); err != nil {
		t.Fatal(err)
	}
	if h := c.History(); len(h) != 1 || h[0].URL != url || h[0].Title != "Test Series" || h[0].Open {
		t.Fatalf("after the load, history = %+v", h)
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)

	h := c.History()
	want := []HistoryChapter{{Label: "Chapter 1", Done: true}, {Label: "Chapter 2"}, {Label: "Chapter 3", Done: true}}
	if len(h) != 1 || !h[0].Open || !reflect.DeepEqual(h[0].Chapters, want) {
		t.Fatalf("history = %+v, want chapters %+v", h, want)
	}
	c.saveHistoryNow()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "open: yes\ndone: Chapter 1\ntodo: Chapter 2\ndone: Chapter 3\n") {
		t.Errorf("history.txt =\n%s", raw)
	}
}

func TestRestoredGroupResumesItsTodo(t *testing.T) {
	c, _, _ := newTestController(t, false, nil, chapters(4, 1)...)
	url := "https://example.com/manga/test"
	c.SetHistory("", []HistoryEntry{
		{Title: "Test Series", URL: url, Open: true, Chapters: []HistoryChapter{{Label: "Chapter 1", Done: true}, {Label: "Chapter 2"}, {Label: "Chapter 4"}}},
		{Title: "Closed", URL: "https://example.com/manga/closed", Chapters: []HistoryChapter{{Label: "Chapter 1"}}},
	})
	groups := c.Groups()
	if len(groups) != 1 || !groups[0].Restored || groups[0].URL != url || groups[0].Running {
		t.Fatalf("groups = %+v", groups)
	}
	var states []JobState
	for _, j := range groups[0].Jobs {
		states = append(states, j.State)
	}
	if !reflect.DeepEqual(states, []JobState{JobDone, JobTodo, JobTodo}) {
		t.Errorf("rows = %v", states)
	}

	id := c.ResumeCandidate()
	if id != groups[0].ID {
		t.Fatalf("ResumeCandidate = %d, want %d", id, groups[0].ID)
	}
	got, load, err := c.Resume(id)
	if err != nil || got != url || !load {
		t.Fatalf("Resume = %q, %v, %v", got, load, err)
	}
	if err := c.Load(url); err != nil {
		t.Fatal(err)
	}
	if sel := c.SelectedIndices(); !reflect.DeepEqual(sel, []int{1, 3}) {
		t.Errorf("selected %v, want the to-do chapters [1 3]", sel)
	}
	if c.Running() {
		t.Error("resuming started a download")
	}

	// Download takes the restored group's place
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	if groups := c.Groups(); len(groups) != 1 || groups[0].Restored {
		t.Errorf("groups after Download = %+v", groups)
	}
	h := c.History()
	want := []HistoryChapter{{Label: "Chapter 1", Done: true}, {Label: "Chapter 2", Done: true}, {Label: "Chapter 4", Done: true}}
	if !reflect.DeepEqual(h[0].Chapters, want) {
		t.Errorf("chapters = %+v", h[0].Chapters)
	}
}

func TestClearClosesTheRestoredSeries(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	c.closeBrowser = func() {}
	c.SetHistory("", []HistoryEntry{{Title: "A", URL: "https://a.example/1", Open: true, Chapters: []HistoryChapter{{Label: "Chapter 1"}}}})
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if len(c.Groups()) != 0 || c.History()[0].Open {
		t.Errorf("after Clear: groups %+v, history %+v", c.Groups(), c.History())
	}
	c.ClearHistory()
	if len(c.History()) != 0 {
		t.Errorf("ClearHistory left %+v", c.History())
	}
}

// ratedSite is a fake site that rates its title
type ratedSite struct {
	*fakeSite
	age int
}

func (r ratedSite) AgeLimit() int { return r.age }

func TestAdultTitleIsMaskedInTheLog(t *testing.T) {
	c, site, _ := newTestController(t, false, nil, chapters(1, 1)...)
	site.title = "Secret Title"
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return ratedSite{site, 18}, nil
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if !c.Adult() || !c.History()[0].Adult {
		t.Errorf("Adult() = %v, history %+v", c.Adult(), c.History())
	}
	c.SetAll(true)
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	waitFinished(t, c)
	if !lastGroup(c).Adult {
		t.Error("the group isn't marked adult")
	}
	for _, l := range c.Log() {
		if strings.Contains(l.Text, "Secret Title") {
			t.Errorf("log line shows the adult title: %q", l.Text)
		}
	}
}

func TestSixteenPlusIsNotMasked(t *testing.T) {
	c, site, _ := newTestController(t, false, nil, chapters(1, 1)...)
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return ratedSite{site, 16}, nil
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if c.Adult() {
		t.Error("a 16+ title is masked")
	}
}
