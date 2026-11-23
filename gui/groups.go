// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/ranges"
)

// group is one press of Download: its chapters' rows, its cancel and its
// summary. Groups run at once, each with the settings in force when it started
// (see Start), and report to their own rows through events.
type group struct {
	id    int
	title string
	// url is the URL the group's series was loaded from
	url string
	// deleted is set once Delete series took the series' files: what the
	// group saved is gone, so its rows stay but it no longer marks the list
	deleted bool
	// settings are the pointer the group's series and site read. The window
	// stops writing it when the group starts (see Start).
	settings *grabber.Settings
	// site is the loaded site the group downloads with, shared with any other
	// group of the same load
	site    grabber.Site
	running bool
	// cancelling is set once the group has been asked to stop, so repeated
	// cancels log once
	cancelling bool
	cancel     context.CancelFunc
	// stop is closed when the group is cancelled, and forGood is set once
	// the user let its dedupe delete for good (see permanent.go)
	stop    <-chan struct{}
	forGood bool
	// ended orders the groups that finished (see Controller.lastEnd)
	ended   uint64
	runinfo RunInfo
	rng     string
	// hasDuplicates and hasGroupDuplicates are the loaded list's, as of the
	// start, for the release labels
	hasDuplicates      bool
	hasGroupDuplicates bool
	jobs               []*Job
	byChap             map[*grabber.Chapter]*Job
	bundle             *Job
	failedC            map[*grabber.Chapter]bool
	// the rows of the volumes, by volume, by listed chapter (for a failed
	// fetch) and by fetched chapter
	byVolume  map[*core.Volume]*Job
	listedVol map[grabber.Filterable]*Job
	chapVol   map[*grabber.Chapter]*Job
	// fetching holds the rows of the chapters whose page list is being
	// fetched, by listed chapter, until Fetched moves them to byChap
	fetching map[grabber.Filterable]*Job
	stats    runStats
	summary  *Summary
	// series and skip are what the group downloads with, kept for the
	// chapters added to it while it runs (see queueLocked)
	series *core.Series
	skip   []ranges.Range
	// keys are the chapters in the group's run, by chapKey, and pending the
	// added passes waiting for the one running to end
	keys    map[string]bool
	pending []pass
	// siteKey names the group's site as site-rate-limits.txt does, and
	// flights are its chapters in flight, by fetched chapter and, until
	// fetched, by listing entry (see SiteBars)
	siteKey  string
	flights  map[*grabber.Chapter]*flight
	flightOf map[grabber.Filterable]*flight
	// restored is set on a group read back from history.txt at startup: its
	// rows are the done and to-do chapters of the last session, and it
	// downloads nothing
	restored bool
	// todoIDs are a restored group's to-do chapters' ids (historyID), by
	// row, from the todo-id lines of history.txt
	todoIDs map[*Job]string
	// adult is set when the site rates the series 18+
	adult bool
	// chapters are the listed chapters of the group's run, every pass, and
	// done those of them saved (or already in the folder), by chapKey;
	// listedOf maps a fetched chapter to its listing entry. They're what
	// history.txt records (see recordGroupLocked).
	chapters grabber.Filterables
	done     map[string]bool
	listedOf map[*grabber.Chapter]grabber.Filterable
	// started and failed are the chapters of the run, by chapKey, that
	// began (their page list was asked for) and that failed, for the marks
	// of the chapter list (see ChapterMarks)
	started map[string]bool
	failed  map[string]bool
	// cleanup is what the correlation of the group's folder did, nil until
	// it ran (see events.Correlated)
	cleanup *Cleanup
	// status is what the group is doing that no row shows (the folder's
	// correlation), nil when nothing
	status *Message
}

// Cleanup is what the correlation of a group's folder did when its download
// started (see core.Series.Correlate): the duplicate files moved to the
// Recycle Bin, their size, and the ones that couldn't be moved
type Cleanup struct {
	Removed int
	Bytes   int64
	Failed  int
	// Permanent is set when the removed files were deleted for good, and
	// Kept are the duplicates left as the drive has no Recycle Bin and the
	// user said no
	Permanent bool
	Kept      int
}

// flight is the progress of one chapter of a group from its fetch to its end,
// whatever row it's in (its own, a volume's or the bundle's): Done of Total is
// its pages' download steps, plus their archive steps when it's packed on its
// own. Total is 0 while its page list is being fetched. seq orders the
// chapters by when they started.
type flight struct {
	seq         uint64
	title, tags string
	done, total int64
}

// pass is chapters added to a running group, downloaded once the chapters
// before them are done, and the range a bundle of them is named by
type pass struct {
	chapters grabber.Filterables
	rng      string
}

// chapKey identifies a listed chapter across loads of the same URL: by the
// site's own id for the release when it has one, else by its number, title,
// language and groups, which tell apart the copies of a number
func chapKey(f grabber.Filterable) string {
	c := grabber.ChapterOf(f)
	if c.ID != "" {
		return "id:" + c.ID
	}
	return fmt.Sprintf("%v\x00%s\x00%s\x00%s", c.Number, c.Title, c.Language, strings.Join(c.GetGroups(), "\x00"))
}

// newGroup returns a group with its row maps made
func newGroup() *group {
	return &group{
		byChap:    map[*grabber.Chapter]*Job{},
		failedC:   map[*grabber.Chapter]bool{},
		byVolume:  map[*core.Volume]*Job{},
		listedVol: map[grabber.Filterable]*Job{},
		chapVol:   map[*grabber.Chapter]*Job{},
		fetching:  map[grabber.Filterable]*Job{},
		keys:      map[string]bool{},
		flights:   map[*grabber.Chapter]*flight{},
		flightOf:  map[grabber.Filterable]*flight{},
		done:      map[string]bool{},
		listedOf:  map[*grabber.Chapter]grabber.Filterable{},
		started:   map[string]bool{},
		failed:    map[string]bool{},
	}
}

// GroupState is a copy of a group for the window: its rows in chapter order
// (completion order is what the events arrive in) and, once it ended, its
// summary
type GroupState struct {
	ID         int
	Title      string
	Running    bool
	Cancelling bool
	Run        RunInfo
	Jobs       []Job
	Summary    *Summary
	// Restored is set on a group of the last session (see group.restored),
	// URL is its series' URL, and Adult is set when the site rates it 18+
	Restored bool
	URL      string
	Adult    bool
	// Unfinished counts the chapters Restart would download again for the
	// whole group (see Controller.Restart)
	Unfinished int
	// Cleanup is what the correlation of the group's folder did, nil when it
	// didn't run
	Cleanup *Cleanup
	// Status is what the group is doing that no row shows, nil when nothing
	Status *Message
}

// siteInUseLocked reports whether a running group downloads with site. The
// caller holds c.mu.
func (c *Controller) siteInUseLocked(site grabber.Site) bool {
	for _, g := range c.groups {
		if g.running && g.site == site {
			return true
		}
	}
	return false
}

// runningLocked reports whether any group runs. The caller holds c.mu.
func (c *Controller) runningLocked() bool {
	for _, g := range c.groups {
		if g.running {
			return true
		}
	}
	return false
}

// seriesGroupLocked returns the running group of the loaded series (of this
// load, or of an earlier load of its URL), nil when none runs. The caller
// holds c.mu.
func (c *Controller) seriesGroupLocked() *group {
	if c.series == nil {
		return nil
	}
	for _, g := range c.groups {
		if g.running && (g.site == c.series.Site || g.url == c.url) {
			return g
		}
	}
	return nil
}

// selectedKeysLocked returns the selected chapters by chapKey. The caller
// holds c.mu.
func (c *Controller) selectedKeysLocked() map[string]bool {
	keys := map[string]bool{}
	for i, chap := range c.series.Chapters {
		if c.selected[i] {
			keys[chapKey(chap)] = true
		}
	}
	return keys
}

// rememberStartLocked records the selection Download was pressed with, for
// DownloadMode. The caller holds c.mu.
func (c *Controller) rememberStartLocked() {
	if c.started == nil {
		c.started = map[string]map[string]bool{}
	}
	c.started[c.url] = c.selectedKeysLocked()
}

// DownloadMode tells the window what Download would do. addTo is the id of
// the running group of the loaded series, 0 when none runs, and fresh counts
// the selected chapters not in its run yet, which Download adds to it.
// unchanged is set, when no such group runs, while the selection is the one
// Download was last pressed with for this URL.
func (c *Controller) DownloadMode() (addTo, fresh int, unchanged bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil || c.loading {
		return 0, 0, false
	}
	if g := c.seriesGroupLocked(); g != nil {
		for i, chap := range c.series.Chapters {
			if c.selected[i] && !g.keys[chapKey(chap)] {
				fresh++
			}
		}
		return g.id, fresh, false
	}
	last := c.started[c.url]
	keys := c.selectedKeysLocked()
	if len(keys) == 0 || len(keys) != len(last) {
		return 0, 0, false
	}
	for k := range keys {
		if !last[k] {
			return 0, 0, false
		}
	}
	return 0, 0, true
}

// dropFinishedLocked forgets the groups that ended, keeping the running ones.
// The caller holds c.mu.
func (c *Controller) dropFinishedLocked() {
	var kept []*group
	for _, g := range c.groups {
		if g.running {
			kept = append(kept, g)
		}
	}
	c.groups = kept
}

// Groups returns a copy of the groups, in the order they started
func (c *Controller) Groups() []GroupState {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]GroupState, len(c.groups))
	for i, g := range c.groups {
		out[i] = g.state()
		c.restartInfoLocked(g, &out[i])
	}
	return out
}

// state is a copy of the group. The caller holds c.mu.
func (g *group) state() GroupState {
	st := GroupState{ID: g.id, Title: g.title, Running: g.running, Cancelling: g.cancelling, Run: g.runinfo, Restored: g.restored, URL: g.url, Adult: g.adult}
	if g.cleanup != nil {
		cl := *g.cleanup
		st.Cleanup = &cl
	}
	if g.status != nil {
		m := *g.status
		st.Status = &m
	}
	st.Jobs = make([]Job, len(g.jobs))
	for i, j := range g.jobs {
		st.Jobs[i] = *j
	}
	sort.SliceStable(st.Jobs, func(i, j int) bool { return st.Jobs[i].Number < st.Jobs[j].Number })
	if g.summary != nil {
		sum := *g.summary
		sum.Failures = append([]Failure(nil), g.summary.Failures...)
		st.Summary = &sum
	}
	return st
}

// LastSummary returns the outcome of the group that ended last, nil while
// any group runs or when none has ended
func (c *Controller) LastSummary() *Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	var last *group
	for _, g := range c.groups {
		if g.running {
			return nil
		}
		if g.summary != nil && (last == nil || g.ended > last.ended) {
			last = g
		}
	}
	if last == nil {
		return nil
	}
	return last.state().Summary
}

// GroupSummary returns the outcome of group id, nil while it runs or when
// there's no such group
func (c *Controller) GroupSummary(id int) *Summary {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, g := range c.groups {
		if g.id == id {
			return g.state().Summary
		}
	}
	return nil
}

// queueLocked adds the selected chapters that aren't in g's run yet to it,
// as a pass of their own that starts once the chapters before it are done.
// They're downloaded with the group's settings, not the window's. It unlocks
// c.mu.
func (c *Controller) queueLocked(g *group) error {
	var fresh grabber.Filterables
	var numbers []float64
	var selected []bool
	for i, chap := range c.series.Chapters {
		add := c.selected[i] && !g.keys[chapKey(chap)]
		numbers = append(numbers, chap.GetNumber())
		selected = append(selected, add)
		if add {
			fresh = append(fresh, chap)
		}
	}
	if len(fresh) == 0 {
		c.mu.Unlock()
		return uiErr("errors.nothingNew", nil, nil)
	}
	rows := len(fresh)
	if g.runinfo.Bundle {
		rows = 1
	}
	if g.runinfo.Volumes {
		spec, _ := core.ParseVolumes(g.settings.Volumes)
		groups, loose, err := core.GroupVolumes(fresh, spec)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		rows = len(groups) + len(loose)
	}
	for _, chap := range fresh {
		g.keys[chapKey(chap)] = true
	}
	g.pending = append(g.pending, pass{chapters: fresh, rng: bundleRange(numbers, selected)})
	g.runinfo.Chapters += len(fresh)
	g.runinfo.Rows += rows
	g.chapters = append(g.chapters, fresh...)
	c.recordGroupLocked(g)
	c.kickHistory()
	c.rememberStartLocked()
	c.appendMsgLocked(LogInfo, msg("log.queued", map[string]any{"count": len(fresh), "title": g.title}))
	c.mu.Unlock()
	c.emit()
	return nil
}

// savedNow is g's saved count, read under c.mu
func (g *group) savedNow(c *Controller) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return g.stats.saved
}

// endPassLocked settles one pass of g's run: n chapters, saved of them kept,
// and Download's file and err. It returns the failures that aren't a chapter's
// own, the bundle written ("" for none) and the chapters a bundle already in
// the folder accounts for, and marks s cancelled or failed. The caller holds c.mu.
func (c *Controller) endPassLocked(g *group, s *Summary, n, saved int, file string, err error) (extra []Failure, bundle string, existing int) {
	var serr *core.SettingError
	switch {
	case errors.As(err, &serr) && serr.Setting == "output-dir":
		// raised before anything was fetched, so no chapter has a result to show
		s.Err = err
		m := msg("log.outputDirFailed", map[string]any{"folder": serr.Value, "error": serr.Err.Error()})
		extra = append(extra, windowFailure("", m))
		c.appendMsgLocked(LogError, m)
	case err != nil && g.runinfo.Volumes && !errors.Is(err, context.Canceled):
		// a volume run's Download fails only on its setting checks (--volumes,
		// the volume filename template), before anything is fetched
		s.Err = err
		m := msg("log.volumesFailed", map[string]any{"reason": err.Error()})
		extra = append(extra, windowFailure("", m))
		c.appendMsgLocked(LogError, m)
	case errors.Is(err, context.Canceled):
		// a cancelled bundle is never packed: the chapters it never started
		// would be silently missing from it
		s.Cancelled = true
	case errors.Is(err, core.ErrExists):
		// the bundle is named by Download's file; none of its chapters was fetched
		existing = n
		m := msg("job.reason.exists", map[string]any{"name": file})
		if g.bundle != nil {
			g.bundle.setReason(m)
		}
		c.appendMsgLocked(LogInfo, msg("log.exists", map[string]any{"name": file}))
	case err != nil:
		s.Err = err
		m := msg("log.packingBundle", map[string]any{"reason": err.Error()})
		extra = append(extra, windowFailure("", m))
		c.appendMsgLocked(LogError, m)
	case g.bundle != nil && saved == 0:
		// core still packs an empty bundle when every chapter failed (as the
		// CLI does), so the file it leaves behind is not a result
		s.Err = uiErr("errors.noChapterDownloaded", nil, nil)
		m := msg("log.bundleEmpty", nil)
		extra = append(extra, windowFailure("", m))
		c.appendMsgLocked(LogError, m)
	default:
		bundle = file
	}
	if g.bundle != nil {
		switch {
		case bundle != "":
			g.bundle.State = JobDone
			g.bundle.Done = g.bundle.Total
		case errors.Is(err, core.ErrExists):
			g.bundle.State = JobSkipped
		default:
			g.bundle.State = JobFailed
		}
		g.bundle.clearStatus()
	}
	return extra, bundle, existing
}

// CancelGroup is Cancel for group id alone
func (c *Controller) CancelGroup(id int) {
	c.cancelGroups(func(g *group) bool { return g.id == id })
}

// cancelGroups cancels the running groups that pick selects and weren't
// cancelled yet, logging once for them
func (c *Controller) cancelGroups(pick func(*group) bool) {
	c.mu.Lock()
	var cancels []context.CancelFunc
	for _, g := range c.groups {
		if g.running && !g.cancelling && g.cancel != nil && pick(g) {
			g.cancelling = true
			cancels = append(cancels, g.cancel)
		}
	}
	if len(cancels) > 0 {
		c.appendMsgLocked(LogInfo, msg("log.cancelling", nil))
	}
	c.mu.Unlock()
	if len(cancels) == 0 {
		return
	}
	for _, cancel := range cancels {
		cancel()
	}
	c.emit()
}
