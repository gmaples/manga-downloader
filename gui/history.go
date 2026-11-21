// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"bufio"
	"bytes"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// historyMax is how many series history.txt keeps, newest first
const historyMax = 100

// historyTime is how history.txt writes a time, in local time
const historyTime = "2006-01-02 15:04"

// HistoryEntry is one series in history.txt: the series loaded or downloaded,
// and what the Activity pane showed of it
type HistoryEntry struct {
	Title string
	URL   string
	// Adult is set when the site rates the title 18+ (grabber.AgeRated), so
	// the window masks it
	Adult bool
	When  time.Time
	// Open is set while the series has a group in the Activity pane; the
	// next launch shows it there again
	Open bool
	// Chapters are the series' chapters a download took, in the order they
	// were added, by their row's label
	Chapters []HistoryChapter
}

// HistoryChapter is a chapter of a HistoryEntry: its label as the Activity
// pane names it (title and tags), and whether its file was saved
type HistoryChapter struct {
	Label string
	Done  bool
}

// Todo counts the chapters not saved yet
func (e HistoryEntry) Todo() int {
	n := 0
	for _, ch := range e.Chapters {
		if !ch.Done {
			n++
		}
	}
	return n
}

// addChapters merges a download's chapters in: a chapter already listed keeps
// its place and stays done once it was, a new one is added at the end
func (e *HistoryEntry) addChapters(chapters []HistoryChapter) {
	at := map[string]int{}
	for i, ch := range e.Chapters {
		at[ch.Label] = i
	}
	for _, ch := range chapters {
		if i, ok := at[ch.Label]; ok {
			e.Chapters[i].Done = e.Chapters[i].Done || ch.Done
			continue
		}
		at[ch.Label] = len(e.Chapters)
		e.Chapters = append(e.Chapters, ch)
	}
}

// historySite is the site's home page of a series URL, "" when it has none
func historySite(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// historyPath is history.txt, next to gui.json
func historyPath(prefsPath string) string {
	return filepath.Join(filepath.Dir(prefsPath), "history.txt")
}

const historyHeader = `# Manga Downloader history: the series you loaded or downloaded, newest first.
# The app keeps the last 100 and rewrites this file as it runs, so edit it with the app closed.
# open: yes shows the series in the Activity pane again at the next start.
# done and todo are its chapters, one per line, as the Activity pane names them.
`

// formatHistory writes entries as history.txt: plain "key: value" lines, a
// blank line between series
func formatHistory(entries []HistoryEntry) []byte {
	var b bytes.Buffer
	b.WriteString(historyHeader)
	yesNo := map[bool]string{true: "yes", false: "no"}
	for _, e := range entries {
		b.WriteString("\n")
		b.WriteString("title: " + oneLine(e.Title) + "\n")
		b.WriteString("url: " + oneLine(e.URL) + "\n")
		b.WriteString("adult: " + yesNo[e.Adult] + "\n")
		if !e.When.IsZero() {
			b.WriteString("when: " + e.When.Local().Format(historyTime) + "\n")
		}
		b.WriteString("open: " + yesNo[e.Open] + "\n")
		for _, ch := range e.Chapters {
			key := "todo"
			if ch.Done {
				key = "done"
			}
			b.WriteString(key + ": " + oneLine(ch.Label) + "\n")
		}
	}
	return b.Bytes()
}

// oneLine keeps a value on its line
func oneLine(s string) string {
	return strings.Join(strings.Fields(strings.NewReplacer("\r", " ", "\n", " ").Replace(s)), " ")
}

// parseHistory reads history.txt. A "title:" line or a blank line starts a
// series; lines starting with # and unknown keys are ignored, and a series
// without a url is dropped.
func parseHistory(r io.Reader) []HistoryEntry {
	var out []HistoryEntry
	var cur *HistoryEntry
	flush := func() {
		if cur != nil && cur.URL != "" && len(out) < historyMax {
			out = append(out, *cur)
		}
		cur = nil
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		if key == "title" && cur != nil && (cur.Title != "" || cur.URL != "") {
			flush()
		}
		if cur == nil {
			cur = &HistoryEntry{}
		}
		switch key {
		case "title":
			cur.Title = value
		case "url":
			cur.URL = value
		case "adult":
			cur.Adult = isYes(value)
		case "when":
			if t, err := time.ParseInLocation(historyTime, value, time.Local); err == nil {
				cur.When = t
			}
		case "open":
			cur.Open = isYes(value)
		case "done", "todo":
			if value != "" {
				cur.Chapters = append(cur.Chapters, HistoryChapter{Label: value, Done: key == "done"})
			}
		}
	}
	flush()
	return out
}

func isYes(s string) bool {
	switch strings.ToLower(s) {
	case "yes", "y", "true", "1", "on":
		return true
	}
	return false
}

// loadHistory reads history.txt at path; a missing or unreadable file is an
// empty history
func loadHistory(path string) []HistoryEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return parseHistory(f)
}

// saveHistory writes entries to path atomically, as saveOptions writes gui.json
func saveHistory(path string, entries []HistoryEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // a no-op once renamed

	if _, err := tmp.Write(formatHistory(entries)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// adultMask is what the log writes for the title of a series the site rates
// 18+; the window masks the titles it shows itself, and reveals them on a click
const adultMask = "[Adult Title]"

// addAdultTitleLocked has the log mask title from now on. The caller holds c.mu.
func (c *Controller) addAdultTitleLocked(title string) {
	if strings.TrimSpace(title) == "" {
		return
	}
	if c.adultTitles == nil {
		c.adultTitles = map[string]bool{}
	}
	c.adultTitles[title] = true
}

// maskLineLocked replaces the adult titles in a log line, its text and its
// message's arguments, with adultMask: file and folder names carry the title
// too. The caller holds c.mu.
func (c *Controller) maskLineLocked(line LogLine) LogLine {
	if len(c.adultTitles) == 0 {
		return line
	}
	mask := func(s string) string {
		for title := range c.adultTitles {
			s = strings.ReplaceAll(s, title, adultMask)
		}
		return s
	}
	line.Text = mask(line.Text)
	if line.Msg != nil {
		m := *line.Msg
		m.Text = mask(m.Text)
		if m.Args != nil {
			args := make(map[string]any, len(m.Args))
			for k, v := range m.Args {
				if s, ok := v.(string); ok {
					v = mask(s)
				}
				args[k] = v
			}
			m.Args = args
		}
		line.Msg = &m
	}
	return line
}

// SetHistory gives the controller history.txt's entries and the path it saves
// them to ("" for nowhere), puts the series that were in the Activity pane
// back there, and starts the saver. Call it once, before the window opens.
func (c *Controller) SetHistory(path string, entries []HistoryEntry) {
	c.mu.Lock()
	c.history = entries
	c.historyPath = path
	c.restoreLocked()
	c.mu.Unlock()
	if path == "" {
		return
	}
	c.histKick = make(chan struct{}, 1)
	go func() {
		for range c.histKick {
			c.saveHistoryNow()
			// the saves of a run's chapters coalesce into one a second
			time.Sleep(time.Second)
		}
	}()
}

// restoreLocked adds a group for each open series of the history, oldest
// first as groups start: its saved chapters done, the others to do. The
// caller holds c.mu.
func (c *Controller) restoreLocked() {
	for i := len(c.history) - 1; i >= 0; i-- {
		e := c.history[i]
		if !e.Open {
			continue
		}
		g := newGroup()
		c.lastID++
		g.id = c.lastID
		g.title, g.url, g.adult, g.restored = e.Title, e.URL, e.Adult, true
		if e.Adult {
			c.addAdultTitleLocked(e.Title)
		}
		for n, ch := range e.Chapters {
			j := &Job{Title: ch.Label, Number: float64(n), State: JobTodo}
			if ch.Done {
				j.State = JobDone
				j.Total, j.Done = 1, 1
			} else {
				j.setReason(msg("job.reason.todo", nil))
			}
			g.jobs = append(g.jobs, j)
		}
		g.runinfo = RunInfo{Chapters: len(e.Chapters), Rows: len(e.Chapters)}
		c.groups = append(c.groups, g)
	}
}

// dropRestoredLocked forgets the restored groups of url, which a new
// download of the series takes the place of. The caller holds c.mu.
func (c *Controller) dropRestoredLocked(url string) {
	kept := c.groups[:0]
	for _, g := range c.groups {
		if !(g.restored && g.url == url) {
			kept = append(kept, g)
		}
	}
	clear(c.groups[len(kept):])
	c.groups = kept
}

// closeHistoryLocked marks the series without a group in the Activity pane
// as closed, so the next launch doesn't show them. The caller holds c.mu.
func (c *Controller) closeHistoryLocked() {
	open := map[string]bool{}
	for _, g := range c.groups {
		open[g.url] = true
	}
	for i := range c.history {
		if !open[c.history[i].URL] {
			c.history[i].Open = false
		}
	}
}

// touchLocked returns the history's entry of url, moved to the front (added
// when there's none) with the title, rating and time updated. The history
// keeps historyMax entries. The caller holds c.mu.
func (c *Controller) touchLocked(url, title string, adult bool) *HistoryEntry {
	e := HistoryEntry{URL: url}
	for i := range c.history {
		if c.history[i].URL == url {
			e = c.history[i]
			c.history = append(c.history[:i], c.history[i+1:]...)
			break
		}
	}
	e.Title, e.Adult, e.When = title, adult, time.Now()
	c.history = append([]HistoryEntry{e}, c.history...)
	if len(c.history) > historyMax {
		c.history = c.history[:historyMax]
	}
	return &c.history[0]
}

// recordGroupLocked writes group g's chapters, done or to do, into its
// series' history entry and marks it open. A restored group is already
// there. The caller holds c.mu.
func (c *Controller) recordGroupLocked(g *group) {
	if g.restored || g.url == "" {
		return
	}
	var chapters []HistoryChapter
	for _, chap := range g.chapters {
		label := grabber.ChapterLabel(chap) + g.releaseTags(chap)
		chapters = append(chapters, HistoryChapter{Label: label, Done: g.done[chapKey(chap)]})
	}
	e := c.touchLocked(g.url, g.title, g.adult)
	e.Open = true
	e.addChapters(chapters)
}

// markDone records chapter ch, fetched or listed, as saved. The caller holds c.mu.
func (g *group) markDone(ch *grabber.Chapter) {
	g.done[g.keyOf(ch)] = true
}

// keyOf is the chapKey of chapter ch, fetched or listed: a fetched one is
// known by its listing entry, as its title can differ. The caller holds c.mu.
func (g *group) keyOf(ch *grabber.Chapter) string {
	if listed := g.listedOf[ch]; listed != nil {
		return chapKey(listed)
	}
	return chapKey(ch)
}

// markVolumeDone records the chapters of volume v as saved. The caller holds c.mu.
func (g *group) markVolumeDone(v *core.Volume) {
	for _, chap := range v.Chapters {
		g.done[chapKey(chap)] = true
	}
}

// kickHistory asks the saver for a save of the history, without waiting
func (c *Controller) kickHistory() {
	if c.histKick == nil {
		return
	}
	select {
	case c.histKick <- struct{}{}:
	default:
	}
}

// saveHistoryNow writes the history to history.txt. Saves run one at a time,
// each with the history as it is when it starts, so the last one wins.
func (c *Controller) saveHistoryNow() {
	c.histSave.Lock()
	defer c.histSave.Unlock()
	c.mu.Lock()
	path := c.historyPath
	entries := make([]HistoryEntry, len(c.history))
	for i, e := range c.history {
		e.Chapters = append([]HistoryChapter(nil), e.Chapters...)
		entries[i] = e
	}
	c.mu.Unlock()
	if path == "" {
		return
	}
	if err := saveHistory(path, entries); err != nil {
		c.addMsg(LogWarn, msg("log.historyFailed", map[string]any{"reason": err.Error()}))
	}
}

// History returns a copy of the history, newest first
func (c *Controller) History() []HistoryEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]HistoryEntry, len(c.history))
	for i, e := range c.history {
		e.Chapters = append([]HistoryChapter(nil), e.Chapters...)
		out[i] = e
	}
	return out
}

// ClearHistory forgets every series of the history, and with them what the
// next launch would show in the Activity pane
func (c *Controller) ClearHistory() {
	c.mu.Lock()
	c.history = nil
	c.mu.Unlock()
	c.kickHistory()
	c.emit()
}

// Resume readies restored group id's to-do chapters for Download: they're
// selected once its series is loaded. It returns the URL to load, and
// whether a load is needed; with the series loaded already, they're
// selected now.
func (c *Controller) Resume(id int) (url string, load bool, err error) {
	c.mu.Lock()
	defer func() {
		c.mu.Unlock()
		c.emit()
	}()
	var g *group
	for _, cand := range c.groups {
		if cand.id == id && cand.restored {
			g = cand
		}
	}
	if g == nil {
		return "", false, uiErr("errors.noSuchGroup", nil, nil)
	}
	if c.loading {
		return "", false, errBusy
	}
	c.resumeURL = g.url
	c.resumeTodo = map[string]bool{}
	for _, j := range g.jobs {
		if j.State == JobTodo {
			c.resumeTodo[j.Title] = true
		}
	}
	if c.series != nil && c.url == g.url && !c.filtersChanged() {
		c.applyResumeLocked(g.url)
		return g.url, false, nil
	}
	return g.url, true, nil
}

// ResumeCandidate is the restored group a launch readies for Download: the
// last one with chapters to do, 0 when there's none
func (c *Controller) ResumeCandidate() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.groups) - 1; i >= 0; i-- {
		g := c.groups[i]
		if !g.restored {
			continue
		}
		for _, j := range g.jobs {
			if j.State == JobTodo {
				return g.id
			}
		}
	}
	return 0
}

// applyResumeLocked selects the to-do chapters Resume readied, when url is
// the series they're of. The caller holds c.mu.
func (c *Controller) applyResumeLocked(url string) {
	if c.resumeURL != url || c.series == nil {
		return
	}
	for i, chap := range c.series.Chapters {
		label := grabber.ChapterLabel(chap) + core.ReleaseTags(chap, c.hasDuplicates, c.hasGroupDuplicates)
		if c.resumeTodo[label] && c.selectable(i) {
			c.selected[i] = true
		}
	}
	c.selGen++
	c.resumeURL, c.resumeTodo = "", nil
}
