// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/elboletaire/manga-downloader/browser"
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/notice"
	"github.com/elboletaire/manga-downloader/ranges"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// maxLogLines caps the log so a long download can't grow it without bound
const maxLogLines = 2000

// errBusy is returned when an action is refused because a series is loading
var errBusy = uiErr("errors.busy", nil, nil)

// LogLevel is the severity of a log line
type LogLevel int

const (
	LogInfo LogLevel = iota
	LogWarn
	LogError
)

// LogLine is one entry of the log. Msg is set when the window translates the
// line; Text is its English rendering.
type LogLine struct {
	Level LogLevel
	Text  string
	Msg   *Message
}

// JobState is where a chapter's download is
type JobState int

const (
	// JobDownloading is a chapter whose pages are being fetched
	JobDownloading JobState = iota
	// JobArchiving is a chapter whose pages are all downloaded and being packed
	JobArchiving
	// JobDone is a chapter that was packed (or, in bundle mode, the bundle)
	JobDone
	// JobFailed is a chapter that couldn't be fetched, downloaded or packed
	JobFailed
	// JobSkipped is a chapter whose pages were all excluded by --skip-pages, or
	// whose file is already in the output folder (--skip-existing)
	JobSkipped
	// JobFetching is a chapter whose page list is being fetched, before any
	// page is known
	JobFetching
	// JobTodo is a chapter of a group restored from history.txt that wasn't
	// saved yet (see restoreLocked)
	JobTodo
)

// Job is the progress of one chapter. In bundle mode there's a single Job for
// the whole bundle, whose Total counts each page's download and archive steps
// (core.DownloadWeight and core.ArchiveWeight, as the CLI's bar does), and with
// volumes one per volume, whose Done is core.VolumeProgress out of a fixed
// Total of volumeScale (its pages aren't known until it's packed).
type Job struct {
	Title string
	// TitleMsg, StatusMsg and ReasonMsg are the texts the GUI wrote, as
	// messages for the window; they sit beside the English Title, Status and Reason
	TitleMsg *Message
	// Tags is the release's language and group (" [en] [Alpha]"), kept apart
	// from Title so a narrow row truncates the title and keeps them
	Tags      string
	Number    float64
	Total     int64
	Done      int64
	State     JobState
	Reason    string
	ReasonMsg *Message
	// Status is the bundle's or the volume's phase ("Downloading
	// <chapter>", "Creating bundle..."); unused for single chapters
	Status    string
	StatusMsg *Message
	// Volume is set on the row of a volume
	Volume bool
	// Key is the row's chapter, by chapKey of its listing entry, or
	// "todo:" and its label on a restored row; "" for a volume or bundle
	Key string
	// Restart is set on a row whose chapter can be downloaded again (see
	// Controller.Restart): one that failed, or a restored one to do
	Restart bool
}

// setTitle, setStatus and setReason set a row's text the GUI wrote, with its message
func (j *Job) setTitle(m Message)  { j.Title, j.TitleMsg = m.Text, &m }
func (j *Job) setStatus(m Message) { j.Status, j.StatusMsg = m.Text, &m }
func (j *Job) setReason(m Message) { j.Reason, j.ReasonMsg = m.Text, &m }

// setReasonErr sets a reason from an error: a shared one the window translates
// keeps its message, any other shows as its text
func (j *Job) setReasonErr(err error) {
	if m := messageOf(err); m != nil {
		j.setReason(*m)
		return
	}
	j.Reason, j.ReasonMsg = err.Error(), nil
}

// clearStatus ends the row's phase
func (j *Job) clearStatus() { j.Status, j.StatusMsg = "", nil }

// RunInfo describes the run in progress, or the last one
type RunInfo struct {
	// Chapters is how many chapters the run was started with
	Chapters int
	Bundle   bool
	// Volumes is set when the run packs one file per volume
	Volumes bool
	// Parts is set on a volumes run that merges each chapter's parts
	Parts bool
	// Rows is how many progress rows the run ends up with: one per chapter,
	// one for a bundle, or one per volume plus one per chapter outside them
	Rows int
}

// Summary is what a finished run produced
type Summary struct {
	Dir       string
	Cancelled bool
	// Saved counts chapters written to disk (or, in bundle mode, kept for the
	// bundle, and with volumes, kept for their volume)
	Saved int
	// Volumes counts the volume files written
	Volumes int
	// Parts is set when those files are chapters with their parts merged
	Parts bool
	// Bundled is set in bundle mode, where Bundle is the bundle's filename
	// (empty when none was written)
	Bundled bool
	Bundle  string
	Skipped int
	// Existing counts the chapters left out because their file was already in
	// the output folder (--skip-existing), kept apart from Skipped; a bundle
	// that was already there counts all its chapters
	Existing int
	Failures []Failure
	// Err is a bundle packing error
	Err error
}

// Failure is a summary line: the release or volume it's about (Label, "" for
// none) and Reason, with ReasonMsg when the window words the reason itself.
type Failure struct {
	Label     string
	Reason    string
	ReasonMsg *Message
}

// newFailure is a failure of label whose reason is err, keeping the message of
// a shared error the window translates
func newFailure(label string, err error) Failure {
	return Failure{Label: label, Reason: err.Error(), ReasonMsg: messageOf(err)}
}

// windowFailure is a failure of label worded by the window: m is its message
func windowFailure(label string, m Message) Failure {
	return Failure{Label: label, Reason: m.Text, ReasonMsg: &m}
}

// runStats collects what the events report during a run, for the summary
type runStats struct {
	saved    int
	volumes  int
	skipped  int
	existing int
	failures []Failure
	bundle   string
	err      error
}

// Controller holds the state of the app and runs the core flow. It knows
// nothing about the window toolkit: the window reads its state through the
// getters and is told about changes through notify, which is called from any
// goroutine and must not call back into the controller.
//
// Every mutation happens under mu; getters return copies, so the view never
// shares memory with the event goroutines.
type Controller struct {
	notify func()
	// identify finds the site for a URL; core.Identify, replaced in tests
	identify func(url string, s *grabber.Settings) (grabber.Site, []error)
	// closeBrowser is browser.Close, replaced in tests
	closeBrowser func()
	// needsVisible is browser.NeedsVisible, and bringToFront
	// browser.BringToFront, replaced in tests
	needsVisible func(url string) bool
	bringToFront func()
	// closing tracks a browser close running in the background (see Clear),
	// which loads wait for rather than meet a browser going away
	closing sync.WaitGroup
	// filterChange is set by BeginLoad when a load changes only the filters of
	// the URL loaded last; load() reads and clears it (see listingFor)
	filterChange bool

	mu sync.Mutex
	// settings is the one pointer the site and core read their settings
	// through, so it's replaced (not mutated) by Load, and mutated by
	// Configure only while nothing is running
	settings *grabber.Settings

	url      string
	series   *core.Series
	loading  bool
	selected []bool
	// hasDuplicates is core.HasDuplicateChapterNumbers for the loaded list, and
	// hasGroupDuplicates is core.HasGroupDuplicates
	hasDuplicates      bool
	hasGroupDuplicates bool
	// loadedLanguage and loadedScanlator are the filters the list was fetched
	// with, which Configure can make stale
	loadedLanguage  string
	loadedScanlator string
	// listings are the unfiltered listings of the session's loads, which a
	// filter change is served from (see listingFor)
	listings listingCache
	// languages and scanlators are the filter options of the site of the last
	// load, nil for each one it doesn't list. They outlive a failed load, so
	// its filters can still be changed.
	languages  *grabber.FilterOptions
	scanlators *grabber.FilterOptions

	// loadErr is the last load's error, which the view shows under the URL
	loadErr error
	// volumesText is the Volumes box, starting empty on each load ("" is the
	// site's own volumes). siteVolumes is those volumes as ranges, "" when they
	// can't be written so; listedVolumes counts the volumes the site lists.
	volumesText   string
	siteVolumes   string
	listedVolumes int
	// update is the newer release the update check found, nil when none
	update *Update
	// updateChecking is set while an update check runs (see claimUpdateCheck), so
	// two checks don't start at once
	updateChecking bool
	// prefs are the app-only preferences, outside settings (see PrefsDTO)
	prefs PrefsDTO
	// systemLocale is the system's locale, as the window reported it, normalized (see SetSystemLocale)
	systemLocale string
	// attention is the browser window waiting for the user (a check to pass
	// there), nil when none is or the user dismissed it
	attention *browser.Attention
	// selGen and logGen count changes to the selection and the log, so a view
	// that pushes state can skip re-sending either when it hasn't moved
	selGen uint64
	logGen uint64

	log []LogLine
	// forkState holds the fields the fork adds (see fork_state.go)
	forkState
}

// NewController returns a controller that works on its own copy of settings
// and routes the library's warnings into its log.
func NewController(settings *grabber.Settings, notifyFn func()) *Controller {
	if notifyFn == nil {
		notifyFn = func() {}
	}
	c := &Controller{
		notify:       notifyFn,
		identify:     core.Identify,
		closeBrowser: browser.Close,
		needsVisible: browser.NeedsVisible,
		bringToFront: browser.BringToFront,
		settings:     settings,
		prefs:        defaultPrefs(),
	}
	c.hold = browser.Hold
	notice.SetHandler(func(level notice.Level, msg string) {
		lvl := LogInfo
		if level == notice.LevelWarn {
			lvl = LogWarn
		}
		if m, ok := noticeMessage(msg); ok {
			c.addMsg(lvl, m)
			return
		}
		c.addLog(lvl, msg)
	})
	browser.SetAttentionHandler(c.onAttention)
	return c
}

// onAttention keeps the browser's attention requests, which the view shows
// over the window until the page renders or its load ends
func (c *Controller) onAttention(a browser.Attention) {
	c.mu.Lock()
	if a.Active {
		c.attention = &a
		c.appendMsgLocked(LogWarn, alertMessage(a))
	} else {
		c.attention = nil
	}
	c.mu.Unlock()
	c.emit()
}

// BrowserAlert is the alert of the browser window waiting for the user, nil
// when none is
func (c *Controller) BrowserAlert() *Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.attention == nil {
		return nil
	}
	m := alertMessage(*c.attention)
	return &m
}

// alertMessage is the browser's alert, in English as browser.Attention words it
func alertMessage(a browser.Attention) Message {
	if a.Browser != "" {
		return msg("browser.alertNamed", map[string]any{"browser": a.Browser, "host": a.Host})
	}
	return msg("browser.alert", map[string]any{"host": a.Host})
}

// DismissBrowserAlert hides the alert; the window stays open, and the load
// goes on waiting for it
func (c *Controller) DismissBrowserAlert() {
	c.mu.Lock()
	had := c.attention != nil
	c.attention = nil
	c.mu.Unlock()
	if had {
		c.emit()
	}
}

// ShowBrowser brings the browser window to the front again
func (c *Controller) ShowBrowser() {
	c.bringToFront()
}

// BrowserVisibleAuto reports whether the browser opens visible on its own for
// the site of the URL loaded: the site needs it (declared by its grabber, or
// escalated earlier in this session), whatever the option says
func (c *Controller) BrowserVisibleAuto() bool {
	c.mu.Lock()
	url := c.url
	c.mu.Unlock()
	return url != "" && c.needsVisible(url)
}

// emit tells the view something changed. Callers must not hold mu.
func (c *Controller) emit() { c.notify() }

// addLog appends a line and notifies. Callers must not hold mu.
func (c *Controller) addLog(level LogLevel, text string) {
	c.mu.Lock()
	c.appendLogLocked(level, text)
	c.mu.Unlock()
	c.emit()
}

// addMsg is addLog for a translatable line
func (c *Controller) addMsg(level LogLevel, m Message) {
	c.mu.Lock()
	c.appendMsgLocked(level, m)
	c.mu.Unlock()
	c.emit()
}

func (c *Controller) appendLogLocked(level LogLevel, text string) {
	c.appendLineLocked(LogLine{Level: level, Text: text})
}

// appendMsgLocked is appendLogLocked for a translatable line
func (c *Controller) appendMsgLocked(level LogLevel, m Message) {
	c.appendLineLocked(LogLine{Level: level, Text: m.Text, Msg: &m})
}

func (c *Controller) appendLineLocked(line LogLine) {
	line = c.maskLineLocked(line)
	c.writeLogFileLocked(line)
	c.logGen++
	c.log = append(c.log, line)
	if len(c.log) > maxLogLines {
		c.log = c.log[len(c.log)-maxLogLines:]
	}
}

// Configure applies fn to the settings. It's refused while a series loads, so
// the load never sees a write. A running group isn't touched: it reads the
// settings it started with (see Start).
func (c *Controller) Configure(fn func(*grabber.Settings)) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	fn(c.settings)
	c.mu.Unlock()
	c.emit()
	return nil
}

// Settings returns a copy of the current settings
func (c *Controller) Settings() grabber.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return *c.settings
}

// Prefs returns a copy of the app-only preferences
func (c *Controller) Prefs() PrefsDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.prefs
}

// SetPrefs applies fn to the preferences. Unlike Configure it's never refused,
// as the preferences don't change a load or a run.
func (c *Controller) SetPrefs(fn func(*PrefsDTO)) {
	c.mu.Lock()
	fn(&c.prefs)
	c.mu.Unlock()
	c.emit()
}

// Load identifies the site for url and fetches its title and chapter list.
// It replaces any loaded series and its selection.
func (c *Controller) Load(url string) error {
	if err := c.BeginLoad(url); err != nil {
		return err
	}
	return c.FinishLoad(url)
}

// BeginLoad marks a load as started, synchronously, so the view can disable
// its controls before the slow part (FinishLoad) runs on another goroutine.
func (c *Controller) BeginLoad(url string) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	c.beginLoadLocked(url)
	c.mu.Unlock()
	c.emit()
	return nil
}

// ConfigureLoad is Configure then BeginLoad in one step, so the window never shows a
// picked filter as not applied.
func (c *Controller) ConfigureLoad(fn func(*grabber.Settings), url string) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	fn(c.settings)
	c.beginLoadLocked(url)
	c.mu.Unlock()
	c.emit()
	return nil
}

// beginLoadLocked starts a load of url with the settings as they are now. The
// caller holds mu and has checked that nothing is loading.
func (c *Controller) beginLoadLocked(url string) {
	c.loading = true
	c.loadErr = nil
	// a fresh settings pointer: the previous series keeps its own, and no
	// Configure can run while this load reads it
	s := new(grabber.Settings)
	*s = *c.settings
	c.settings = s
	// a load of the URL of the last one with other filters is a filter change:
	// it's served from the listings it can be (see listingFor). Any other load
	// is an explicit reload, which fetches.
	c.filterChange = url == c.url && (s.Language != c.loadedLanguage || s.Scanlator != c.loadedScanlator)
	c.url = url
}

// abortLoad ends a load that panicked, so the window isn't left locked
// waiting for it. It's the same outcome as FinishLoad failing.
func (c *Controller) abortLoad(err error) {
	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	c.series = nil
	c.selected = nil
	c.selGen++
	c.mu.Unlock()
	c.emit()
}

// FinishLoad does the work BeginLoad started. Call it exactly once after a
// successful BeginLoad.
func (c *Controller) FinishLoad(url string) error {
	c.mu.Lock()
	s := c.settings
	c.mu.Unlock()

	// a browser still closing after Clear would fail the load
	c.closing.Wait()
	// the browser's visibility is read when it starts, and loading a site
	// may be what starts it
	browser.SetVisible(s.BrowserVisible)
	release := c.hold()
	// a loaded series keeps its hold, so its download reuses this browser
	kept := false
	defer func() {
		if !kept {
			release()
		}
	}()
	err := c.load(url, s)

	c.mu.Lock()
	c.loading = false
	c.loadErr = err
	c.noteVPNLocked(ratelimit.SiteKey(url), err)
	if err != nil {
		c.series = nil
		c.selected = nil
		c.selGen++
	}
	// the series loaded before this one, if any, is gone: so is its hold
	prev := c.loadRelease
	c.loadRelease = nil
	if err == nil {
		c.loadRelease = release
		kept = true
	}
	start := c.startPending && err == nil
	c.startPending = false
	if err != nil && c.resumeURL == url {
		// a restart whose series didn't load is dropped, not kept for a later load
		c.resumeStart, c.resumeKeys, c.resumeIDs = false, nil, nil
	}
	c.mu.Unlock()
	if prev != nil {
		prev()
	}
	c.emit()
	if start {
		c.startRestart()
	}
	return err
}

func (c *Controller) load(url string, s *grabber.Settings) error {
	// the filter change BeginLoad recorded is taken here, so it never bleeds
	// into a later load. A Refilterable site of the loaded series is reused on a
	// filter change that isn't cached, as it filters what it already fetched.
	c.mu.Lock()
	filterChange := c.filterChange
	c.filterChange = false
	var reuse grabber.Site
	// a site a running group downloads with keeps its settings: it isn't
	// repointed at this load's
	if filterChange && c.series != nil && !c.siteInUseLocked(c.series.Site) {
		if _, ok := c.series.Site.(grabber.Refilterable); ok {
			reuse = c.series.Site
		}
	}
	c.mu.Unlock()

	listing, site, err := c.listingFor(url, s, reuse, filterChange)
	if site == nil {
		c.mu.Lock()
		c.languages, c.scanlators = nil, nil
		c.mu.Unlock()
		return err
	}

	var series *core.Series
	if err == nil {
		series, err = listing.Series(s)
	}
	if err != nil {
		// the reload gets the listing of its own filters (dropUnmatchedFilters'
		// settings): cached when the listing of the URL is, else fetched again
		// (the failed site may have read its filters), unless it's Refilterable
		if reload, dropLanguage := c.dropUnmatchedFilters(site, s, err); reload {
			// the site keeps the pointer it's identified with, and Configure
			// changes c.settings: a copy would leave the download ignoring
			// later option changes. So the language is cleared in place just
			// for the retry (nothing else touches s while a load runs).
			typed := s.Language
			if dropLanguage {
				c.mu.Lock()
				s.Language = ""
				c.mu.Unlock()
			}
			var retryReuse grabber.Site
			if _, ok := site.(grabber.Refilterable); ok {
				retryReuse = site
			}
			retry, retrySite, retryErr := c.listingFor(url, s, retryReuse, true)
			if retrySite != nil {
				site, err, series = retrySite, retryErr, nil
				if err == nil {
					series, err = retry.Series(s)
				}
			}
			if dropLanguage {
				c.mu.Lock()
				s.Language = typed
				c.mu.Unlock()
			}
		}
	}

	// the options are kept whether or not the load worked; a series already
	// carries them, and asking the site again costs requests
	var languages, scanlators *grabber.FilterOptions
	if series != nil {
		languages, scanlators = series.Languages, series.Scanlators
	} else {
		languages, scanlators = core.FilterOptionsOf(site, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.languages, c.scanlators = languages, scanlators
	// a value typed in another case takes the site's spelling, so the options
	// saved, the loaded filters and the selector all agree on it
	s.Language = listedSpelling(s.Language, languages)
	s.Scanlator = listedSpelling(s.Scanlator, scanlators)
	// a language dropped for this load on a site that lists none is recorded as
	// typed: no selector shows it, and the field would read as stale otherwise
	c.loadedLanguage = s.Language
	c.loadedScanlator = s.Scanlator
	if err != nil {
		return err
	}
	c.series = series
	c.selGen++
	c.selected = make([]bool, len(series.Chapters))
	c.hasDuplicates = core.HasDuplicateChapterNumbers(series.Chapters)
	c.hasGroupDuplicates = core.HasGroupDuplicates(series.Chapters)
	c.siteVolumes = core.SiteVolumes(series.Chapters)
	c.listedVolumes = core.ListedVolumes(series.Chapters)
	c.volumesText = ""
	c.adult = false
	if r, ok := series.Site.(grabber.AgeRated); ok {
		c.adult = r.AgeLimit() >= grabber.AgeAdult
	}
	if c.adult {
		c.addAdultTitleLocked(series.Title)
	}
	c.touchLocked(url, series.Title, c.adult)
	c.applyResumeLocked(url)
	c.kickHistory()
	c.appendMsgLocked(LogInfo, msg("log.loaded", map[string]any{"title": series.Title, "count": len(series.Chapters)}))
	return nil
}

// listingFor returns the unfiltered listing of url and its site, nil when no site
// matches. With useCache, a listing cached for the URL, language (and group for a
// ScanlatorLister) is reused. The site comes back even on a fetch error.
func (c *Controller) listingFor(url string, s *grabber.Settings, reuse grabber.Site, useCache bool) (*core.Listing, grabber.Site, error) {
	c.mu.Lock()
	if useCache {
		// a cached listing whose site a running group uses can't be served: its
		// site would be repointed at this load's settings
		if listing := c.listings.get(url, s); listing != nil && !c.siteInUseLocked(listing.Site) {
			c.mu.Unlock()
			setSiteSettings(listing.Site, s)
			return listing, listing.Site, nil
		}
	} else {
		// an explicit reload replaces what the URL had cached, even when it fails
		c.listings.drop(url, s)
	}
	c.mu.Unlock()

	site := reuse
	if site != nil {
		setSiteSettings(site, s)
	} else {
		var siteErrs []error
		site, siteErrs = c.identify(url, s)
		// what was tried on an unknown site is the load's error, not a check
		var unknown *grabber.NotRecognisedError
		for _, err := range siteErrs {
			if errors.As(err, &unknown) {
				continue
			}
			c.addMsg(LogWarn, msg("log.siteCheck", map[string]any{"reason": err.Error()}))
		}
		if site == nil {
			// the checks' errors stay under the message, for Controller.VPNHint
			checks := errors.Join(siteErrs...)
			if unknown != nil {
				return nil, nil, uiErr("errors.siteUnknownTried", map[string]any{"reason": unknown.Detail()}, checks)
			}
			return nil, nil, uiErr("errors.siteUnknown", nil, checks)
		}
	}

	listing, err := core.Fetch(site)
	if err != nil {
		return nil, site, err
	}
	c.mu.Lock()
	c.listings.put(url, s, listing)
	c.mu.Unlock()

	return listing, site, nil
}

// setSiteSettings points a site at the settings it reads from, as the
// settings pointer changes on each load
func setSiteSettings(site grabber.Site, s *grabber.Settings) {
	if su, ok := site.(interface{ SetSettings(*grabber.Settings) }); ok {
		su.SetSettings(s)
	}
}

// dropUnmatchedFilters handles a load that found nothing for a filter the
// series doesn't have, which the window would otherwise just fail on: each
// such filter is reset to its fallback (see unmatchedFallback) and logged, and
// whether to reload is returned, and whether to do so without the language. It
// only looks at errors that mean "no chapters for this filter": a network
// failure keeps the value.
//
// A language on a site that lists none is dropped for this load only: the
// saved language stays, for the other sites that take it. The caller clears it
// on the shared settings for the reload only and puts it back. Such a site can't say which languages it has, so only
// a NoChaptersError with no scanlator to blame is taken as the language's
// doing: a scanlator the site lists, or none, can't have emptied the list. A
// typed scanlator it doesn't list is reset instead, so the language is kept.
func (c *Controller) dropUnmatchedFilters(site grabber.Site, s *grabber.Settings, err error) (reload, dropLanguage bool) {
	var noChapters *core.NoChaptersError
	var chaptersErr *core.ChaptersError
	var unavailable *grabber.UnavailableFilterError
	// a site that picks the filter while fetching the title fails there
	// instead, with an error of its own
	rejected := errors.As(err, &unavailable)
	if !errors.As(err, &noChapters) && !errors.As(err, &chaptersErr) && !rejected {
		return false, false
	}

	languages, scanlators := core.FilterOptionsOf(site, err)
	// s is the settings the controller holds, so the typed values are read
	// before they're overwritten
	typedLanguage, typedScanlator := s.Language, s.Scanlator
	language, languageReset := unmatchedFallback(typedLanguage, languages)
	scanlator, scanlatorReset := unmatchedFallback(typedScanlator, scanlators)
	scanlatorCleared := typedScanlator == "" || typedScanlator == grabber.FilterAll || listsValue(scanlators, typedScanlator)
	languageDropped := !languageReset && !listsValues(languages) && (noChapters != nil || (rejected && unavailable.Filter == "language")) &&
		typedLanguage != "" && typedLanguage != grabber.FilterAll && scanlatorCleared
	if !languageReset && !scanlatorReset && !languageDropped {
		return false, false
	}

	c.mu.Lock()
	if languageReset {
		c.settings.Language = language
	}
	if scanlatorReset {
		c.settings.Scanlator = scanlator
	}
	c.mu.Unlock()
	if languageReset {
		c.addMsg(LogInfo, fallbackMessage("language", typedLanguage, language))
	}
	if scanlatorReset {
		c.addMsg(LogInfo, fallbackMessage("group", typedScanlator, scanlator))
	}

	if languageDropped {
		c.addMsg(LogInfo, fallbackMessage("language", typedLanguage, ""))
	}

	return true, languageDropped
}

// listsValue reports whether the site offers value for a filter, ignoring case
func listsValue(options *grabber.FilterOptions, value string) bool {
	if !listsValues(options) {
		return false
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return true
		}
	}

	return false
}

// unmatchedFallback is what a typed filter value falls back to when the series
// doesn't offer it: "all" where the site takes it, the site's Current value
// otherwise, and no filter ("") when it has neither. reset is false when
// there's nothing to fall back from: no value, "all" itself, a site that lists
// nothing, or a value the series does offer.
func unmatchedFallback(value string, options *grabber.FilterOptions) (fallback string, reset bool) {
	if value == "" || value == grabber.FilterAll || !listsValues(options) {
		return "", false
	}
	if listsValue(options, value) {
		return "", false
	}
	if options.All {
		return grabber.FilterAll, true
	}

	return options.Current, true
}

// listsValues reports whether the site offers any value for a filter: the
// selector isn't shown otherwise (see newFilter), so nothing can be matched
func listsValues(options *grabber.FilterOptions) bool {
	return options != nil && len(options.Values) > 0
}

// fallbackMessage is the log line for a filter that was reset, naming what
// the load went on with instead. kind is "language" or "group".
func fallbackMessage(kind, value, fallback string) Message {
	args := map[string]any{"value": value, "fallback": fallback}
	group := kind == "group"
	switch {
	case fallback == grabber.FilterAll && group:
		return msg("log.groupFallbackAll", args)
	case fallback == grabber.FilterAll:
		return msg("log.languageFallbackAll", args)
	case fallback == "" && group:
		return msg("log.groupFallbackNone", args)
	case fallback == "":
		return msg("log.languageFallbackNone", args)
	case group:
		return msg("log.groupFallbackValue", args)
	default:
		return msg("log.languageFallbackValue", args)
	}
}

// Filter is what a filter's selector offers for the loaded series
type Filter struct {
	// Values are the values the site lists for the filter
	Values []string
	// All is set when the site also takes "all" (every value, mixed)
	All bool
	// Selected is the value the loaded list shows: the one it was loaded with,
	// or else what the site shows by default (see selectedFilter)
	Selected string
}

// Filters returns the language and scanlator selectors: nil for a filter the
// site doesn't list, or when no site was identified
func (c *Controller) Filters() (language, scanlator *Filter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var chapters grabber.Filterables
	if c.series != nil {
		chapters = c.series.Chapters
	}

	language = newFilter(c.languages, selectedFilter(c.loadedLanguage, c.languages, commonLanguage(chapters)))
	scanlator = newFilter(c.scanlators, selectedFilter(c.loadedScanlator, c.scanlators, ""))

	return language, scanlator
}

// newFilter builds the selector of one filter, nil when the site doesn't list it
func newFilter(options *grabber.FilterOptions, selected string) *Filter {
	if options == nil || len(options.Values) == 0 {
		return nil
	}

	return &Filter{
		Values:   append([]string(nil), options.Values...),
		All:      options.All,
		Selected: selected,
	}
}

// listedSpelling returns the spelling the site lists value under when it
// matches one ignoring case ("delta" is "Delta"), value itself otherwise
func listedSpelling(value string, options *grabber.FilterOptions) string {
	if options == nil {
		return value
	}
	for _, v := range options.Values {
		if strings.EqualFold(v, value) {
			return v
		}
	}

	return value
}

// selectedFilter is the value the loaded list shows for one filter. A value
// the list was loaded with is what it shows, in the site's spelling. With none,
// the site's default shows: Current when the site has one, else the language
// every chapter shares (sites whose default is one language, like "en"), else
// every value when the site takes "all". uniform is commonLanguage, or "" for a
// filter whose chapters don't say.
func selectedFilter(loaded string, options *grabber.FilterOptions, uniform string) string {
	switch {
	case loaded != "" || options == nil:
		return listedSpelling(loaded, options)
	case options.Current != "":
		return options.Current
	case uniform != "":
		return uniform
	case options.All:
		return grabber.FilterAll
	}

	return ""
}

// commonLanguage returns the language every chapter reports, "" when they
// differ or don't report one
func commonLanguage(chapters grabber.Filterables) string {
	lang := ""
	for i, chap := range chapters {
		l, ok := chap.(interface{ GetLanguage() string })
		if !ok {
			return ""
		}
		if i == 0 {
			lang = l.GetLanguage()
		} else if l.GetLanguage() != lang {
			return ""
		}
	}

	return lang
}

// Clear forgets the loaded series: its URL, title, chapters, selection, the
// outcome of its load and the groups that finished (the Activity tab), so the
// window starts from nothing. The options, the log and the running groups stay.
// It's refused while a series loads.
func (c *Controller) Clear() error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	c.url = ""
	c.series = nil
	c.selected = nil
	c.selGen++
	c.hasDuplicates = false
	c.hasGroupDuplicates = false
	c.volumesText, c.siteVolumes, c.listedVolumes = "", "", 0
	c.languages, c.scanlators = nil, nil
	c.loadedLanguage, c.loadedScanlator = "", ""
	// the sites of the cached listings may hold a browser: they go with the series
	c.listings = nil
	c.loadErr = nil
	c.attention = nil
	loadRelease := c.loadRelease
	c.loadRelease = nil
	c.dropFinishedLocked()
	c.closeHistoryLocked()
	c.kickHistory()
	if c.runningLocked() {
		// a running group may still use the browser: it closes when the last
		// group releases it (see browser.Hold), so this release never closes it
		c.mu.Unlock()
		if loadRelease != nil {
			loadRelease()
		}
		c.emit()
		return nil
	}
	// done with the series: its browser (and maybe window) can go. Nothing
	// can be using it, as Clear is refused while loading and no group runs.
	// It's closed in the background, as that can take seconds; added under mu
	// so a load that starts next is sure to wait for it.
	c.closing.Add(1)
	c.mu.Unlock()
	c.emit()
	go func() {
		defer c.closing.Done()
		if loadRelease != nil {
			loadRelease()
		}
		c.closeBrowser()
	}()
	return nil
}

// Stale reports whether the language or scanlator filter changed since the
// loaded list was fetched, which means the list must be reloaded to match. A
// load already running is the reload that applies the change, so it isn't stale.
func (c *Controller) Stale() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil && !c.loading && c.filtersChanged()
}

// filtersChanged reports whether the filters differ from the loaded list's. For
// a scanlator "" and "all" are the same filter; for a language they aren't.
// The caller holds c.mu.
func (c *Controller) filtersChanged() bool {
	scanlator := func(v string) string {
		if strings.EqualFold(v, grabber.FilterAll) {
			return ""
		}

		return v
	}

	return c.settings.Language != c.loadedLanguage || scanlator(c.settings.Scanlator) != scanlator(c.loadedScanlator)
}

// URL is the last URL passed to Load
func (c *Controller) URL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.url
}

// Loaded reports whether a series is loaded
func (c *Controller) Loaded() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil
}

// LoadError is the error of the last load, nil when it succeeded
func (c *Controller) LoadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadErr
}

// Cancelling reports whether every running group has been asked to stop
// (false when none runs)
func (c *Controller) Cancelling() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	some := false
	for _, g := range c.groups {
		if g.running {
			if !g.cancelling {
				return false
			}
			some = true
		}
	}
	return some
}

// SelectionGen changes whenever the selection does (see selGen)
func (c *Controller) SelectionGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selGen
}

// LogGen changes whenever the log does: a line added, or ClearLog (see logGen)
func (c *Controller) LogGen() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logGen
}

// LoadedFilters returns the language and scanlator the loaded list was
// fetched with
func (c *Controller) LoadedFilters() (language, scanlator string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loadedLanguage, c.loadedScanlator
}

// Loading reports whether a series is being loaded
func (c *Controller) Loading() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loading
}

// Running reports whether any group is downloading
func (c *Controller) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runningLocked()
}

// Adult reports whether the site rates the loaded series 18+
func (c *Controller) Adult() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.series != nil && c.adult
}

// Title returns the loaded series' title
func (c *Controller) Title() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return ""
	}
	return c.series.Title
}

// Chapters returns the loaded chapter list, sorted by number. The list is
// never modified after Load, so the slice is shared with the caller.
func (c *Controller) Chapters() grabber.Filterables {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return nil
	}
	return c.series.Chapters
}

// HasDuplicates reports whether the list repeats chapter numbers across
// languages, so the view should show each chapter's language
func (c *Controller) HasDuplicates() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasDuplicates
}

// HasGroupDuplicates reports whether the list repeats a chapter number in one
// language with the groups naming the copies, so the view should show each
// chapter's group
func (c *Controller) HasGroupDuplicates() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasGroupDuplicates
}

// SelectedIndices returns the indices of the selected chapters, in list order,
// read under one lock so a concurrent load can't interleave with it
func (c *Controller) SelectedIndices() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []int{}
	for i, on := range c.selected {
		if on {
			out = append(out, i)
		}
	}
	return out
}

// IsSelected reports whether chapter i of Chapters is selected
func (c *Controller) IsSelected(i int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return i >= 0 && i < len(c.selected) && c.selected[i]
}

// SelectedCount returns how many chapters are selected
func (c *Controller) SelectedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, on := range c.selected {
		if on {
			n++
		}
	}
	return n
}

// selectable reports whether chapter i may be selected: the ones the
// listing marks as locked can't be, as a run could only fail them. Needs c.mu.
func (c *Controller) selectable(i int) bool {
	return !grabber.LockedOf(c.series.Chapters[i])
}

// SetSelected selects or deselects chapter i; a locked chapter is never
// selected. It's refused while a series loads, as
// SetSelectedMany is.
func (c *Controller) SetSelected(i int, on bool) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	if i < 0 || i >= len(c.selected) {
		c.mu.Unlock()
		return nil
	}
	if on && !c.selectable(i) {
		c.mu.Unlock()
		return nil
	}
	c.selected[i] = on
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetSelectedMany selects or deselects the chapters at indices in one step,
// leaving locked ones unselected; one index out of range rejects the whole
// call. It's refused while a series loads, as Configure is.
func (c *Controller) SetSelectedMany(indices []int, on bool) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	for _, i := range indices {
		if i < 0 || i >= len(c.selected) {
			c.mu.Unlock()
			return uiErr("errors.chapterMissing", map[string]any{"index": i}, nil)
		}
	}
	for _, i := range indices {
		c.selected[i] = on && c.selectable(i)
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SetAll selects or deselects every chapter but the locked ones. It's refused
// while a series loads.
func (c *Controller) SetAll(on bool) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	for i := range c.selected {
		c.selected[i] = on && c.selectable(i)
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return nil
}

// SelectRange adds the unlocked chapters whose number falls in the range text
// (e.g. "1-10,12") to the selection and returns how many it selected. It's
// refused while a series loads.
func (c *Controller) SelectRange(text string) (int, error) {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return 0, errBusy
	}
	// parsed under the lock: it's cheap, and the busy check must come first
	rngs, err := ranges.Parse(text)
	if err != nil {
		c.mu.Unlock()
		return 0, err
	}
	n := 0
	if c.series != nil {
		for i, chap := range c.series.Chapters {
			if inRanges(chap.GetNumber(), rngs) && c.selectable(i) {
				c.selected[i] = true
				n++
			}
		}
		c.selGen++
	}
	c.mu.Unlock()
	c.emit()
	return n, nil
}

func inRanges(n float64, rngs []ranges.Range) bool {
	for _, r := range rngs {
		if n >= r.Begin && n <= r.End {
			return true
		}
	}
	return false
}

// Volumes returns the Volumes box, the site's volumes in --volumes' syntax
// (site is "" when they can't be written as ranges) and how many volumes the
// site lists
func (c *Controller) Volumes() (text, site string, listed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.volumesText, c.siteVolumes, c.listedVolumes
}

// SetVolumes sets the Volumes box: chapter ranges, one group per volume
// ("1-7/8-16", "16:112-114"), "auto" or "" for the site's own. A value that doesn't parse
// is refused, as is any change while a series loads.
func (c *Controller) SetVolumes(text string) error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	spec, err := core.ParseVolumes(text)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	// parts is Pack as's Merged, which has no box
	if spec != nil && spec.Parts {
		c.mu.Unlock()
		return uiErr("errors.partsVolumes", nil, nil)
	}
	c.volumesText = strings.TrimSpace(text)
	c.mu.Unlock()
	c.emit()
	return nil
}

// runVolumes is the --volumes value a run in volume mode packs: the box,
// or the site's volumes when it's empty. Callers hold mu.
func (c *Controller) runVolumes() string {
	if c.volumesText == "" {
		return core.VolumesAuto
	}
	return c.volumesText
}

// Log returns a copy of the log
func (c *Controller) Log() []LogLine {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]LogLine(nil), c.log...)
}

// ClearLog empties the log. A running download keeps going: its later lines
// just start the log again.
func (c *Controller) ClearLog() {
	c.mu.Lock()
	c.logGen++
	c.log = nil
	c.mu.Unlock()
	c.emit()
}

// Start validates the settings and starts downloading the selected chapters
// in the background. Validation errors are returned synchronously.
//
// Each Start is a group of its own, which runs alongside the groups already
// running. The group keeps the settings as they are now: the window carries on
// with a copy, so a later Configure never reaches it.
func (c *Controller) Start() error {
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	if c.series == nil {
		c.mu.Unlock()
		return uiErr("errors.loadFirst", nil, nil)
	}
	if c.filtersChanged() {
		c.mu.Unlock()
		return uiErr("errors.filtersChanged", nil, nil)
	}
	// one series downloads once at a time: while it does, of this load or of
	// a reload of its URL, Download adds to its run instead
	if g := c.seriesGroupLocked(); g != nil {
		// the group's run holds the browser, so the load's hold can go
		loadRelease := c.loadRelease
		c.loadRelease = nil
		err := c.queueLocked(g)
		if loadRelease != nil {
			loadRelease()
		}
		return err
	}
	// volume mode packs the Volumes box, which isn't saved with the options;
	// a parts run has no box
	volumes := c.settings.Volumes != ""
	parts := c.settings.Volumes == core.VolumesParts
	if volumes && !parts {
		c.settings.Volumes = c.runVolumes()
	}
	skip, err := core.Validate(c.settings)
	if err != nil {
		c.mu.Unlock()
		return err
	}

	var chapters grabber.Filterables
	var numbers []float64
	var selected []bool
	for i, chap := range c.series.Chapters {
		numbers = append(numbers, chap.GetNumber())
		selected = append(selected, c.selected[i])
		if c.selected[i] {
			chapters = append(chapters, chap)
		}
	}
	if len(chapters) == 0 {
		c.mu.Unlock()
		return uiErr("errors.selectOne", nil, nil)
	}
	// the run's folder: the picked one, or with series folders on, the
	// series' folder inside it, made now, as the archivers make no folders
	outDir := runDir(c.settings, c.series, chapters)
	if outDir != c.settings.OutputDir {
		// made with the controller free, as a card can be slow to answer;
		// a load or another Start of the series meanwhile wins
		series := c.series
		c.mu.Unlock()
		err := mkdirAll(outDir, 0o755)
		c.mu.Lock()
		if err != nil {
			c.mu.Unlock()
			return uiErr("errors.seriesFolder", map[string]any{"path": outDir}, err)
		}
		if c.loading || c.series != series || c.seriesGroupLocked() != nil {
			c.mu.Unlock()
			return errBusy
		}
	}
	rows := len(chapters)
	if c.settings.Bundle {
		rows = 1
	}
	if volumes {
		// grouped here too, so a site without volumes is refused before the
		// run starts rather than failing it
		spec, _ := core.ParseVolumes(c.settings.Volumes)
		groups, loose, err := core.GroupVolumes(chapters, spec)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		rows = len(groups) + len(loose)
	}

	ctx, cancel := context.WithCancel(context.Background())
	g := newGroup()
	c.lastID++
	g.id = c.lastID
	g.title = c.series.Title
	g.url = c.url
	g.site = c.series.Site
	g.cancel = cancel
	g.stop = ctx.Done()
	g.running = true
	g.hasDuplicates, g.hasGroupDuplicates = c.hasDuplicates, c.hasGroupDuplicates
	g.rng = bundleRange(numbers, selected)
	g.runinfo = RunInfo{Chapters: len(chapters), Bundle: c.settings.Bundle, Volumes: volumes, Parts: parts, Rows: rows}
	// the group takes the settings as they are, and the window goes on with a
	// copy. The site, which no other running group uses (see above), reads them.
	settings := c.settings
	g.settings = settings
	c.settings = new(grabber.Settings)
	*c.settings = *settings
	setSiteSettings(g.site, settings)
	if settings.Bundle {
		g.bundle = &Job{}
		g.bundle.setTitle(msg("job.title.bundle", map[string]any{"range": g.rng}))
		g.bundle.setStatus(msg("job.phase.gathering", nil))
		g.jobs = append(g.jobs, g.bundle)
	}
	c.groups = append(c.groups, g)
	series := c.series.WithSettings(settings)
	if outDir != settings.OutputDir {
		// a copy that saves there, so the picked folder stays the option
		series = series.WithOutputDir(outDir)
	}
	rng := g.rng
	g.series, g.skip = series, skip
	g.siteKey = ratelimit.SiteKey(g.site.BaseUrl())
	if g.siteKey == "" {
		g.siteKey = ratelimit.SiteKey(g.url)
	}
	for _, chap := range chapters {
		g.keys[chapKey(chap)] = true
	}
	g.adult = c.adult
	g.chapters = append(g.chapters, chapters...)
	c.dropRestoredLocked(g.url)
	c.recordGroupLocked(g)
	c.kickHistory()
	c.rememberStartLocked()
	c.appendMsgLocked(LogInfo, msg("log.downloading", map[string]any{"count": len(chapters), "title": series.Title}))
	loadRelease := c.loadRelease
	c.loadRelease = nil
	c.mu.Unlock()
	c.emit()

	// the browser's visibility is read when it starts, so it must follow the
	// option as of this run, not the one in force when the series was loaded
	browser.SetVisible(settings.BrowserVisible)
	// Hold keeps the browser open; releasing closes it when the last group is done
	release := c.hold()
	// taken over from the load, after the run's own hold, so the browser the
	// load opened stays open for this run instead of closing and reopening
	if loadRelease != nil {
		loadRelease()
	}
	go func() {
		defer release()
		c.run(ctx, g, series, chapters, skip, rng)
	}()
	return nil
}

// run is the download goroutine of group g. It downloads chapters, then each
// pass added to the group while it runs (see queueLocked), in turn, unless the
// group was cancelled, and sums them up in one summary.
func (c *Controller) run(ctx context.Context, g *group, series *core.Series, chapters grabber.Filterables, skip []ranges.Range, rng string) {
	s := &Summary{
		Dir:     series.OutputDir(),
		Bundled: g.bundle != nil,
		Parts:   g.runinfo.Parts,
	}
	// the outcome of each pass that isn't one of its chapters' (see endPassLocked)
	var extra []Failure
	var bundles []string
	existing := 0
	for {
		saved := g.savedNow(c)
		file, err := series.Download(ctx, chapters, skip, rng, events{c, g})

		c.mu.Lock()
		f, bundle, exists := c.endPassLocked(g, s, len(chapters), g.stats.saved-saved, file, err)
		if bundle != "" || exists > 0 {
			// a bundle saved, or already there, holds every chapter of the pass
			for _, chap := range chapters {
				g.done[chapKey(chap)] = true
			}
		}
		c.recordGroupLocked(g)
		c.kickHistory()
		extra = append(extra, f...)
		if bundle != "" {
			bundles = append(bundles, bundle)
		}
		existing += exists
		if ctx.Err() != nil || len(g.pending) == 0 {
			break
		}
		next := g.pending[0]
		g.pending = g.pending[1:]
		chapters, rng = next.chapters, next.rng
		if g.bundle != nil {
			g.bundle = &Job{}
			g.bundle.setTitle(msg("job.title.bundle", map[string]any{"range": rng}))
			g.bundle.setStatus(msg("job.phase.gathering", nil))
			g.jobs = append(g.jobs, g.bundle)
		}
		c.mu.Unlock()
		c.emit()
	}
	defer func() {
		c.mu.Unlock()
		c.emit()
	}()

	s.Saved = g.stats.saved
	s.Volumes = g.stats.volumes
	s.Skipped = g.stats.skipped
	s.Existing = g.stats.existing + existing
	s.Failures = append(g.stats.failures, extra...)
	s.Bundle = strings.Join(bundles, ", ")
	g.summary = s
	g.running = false
	g.pending = nil
	clear(g.flights)
	clear(g.flightOf)
	c.lastEnd++
	g.ended = c.lastEnd
	// the group's context is released with it
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
}

// Cancel stops every running group from starting new chapters. Chapters
// already in flight finish. It's a no-op when nothing runs or every group was
// cancelled already.
func (c *Controller) Cancel() {
	c.cancelGroups(func(*group) bool { return true })
}

// mkdirAll is os.MkdirAll; tests slow it down
var mkdirAll = os.MkdirAll

// shutdownWait bounds how long quitting waits for a run to wind down and for
// the browser to close, so a stuck render can't hang the window's exit
const shutdownWait = 3 * time.Second

// Shutdown cancels any run and closes the browser, so Chrome doesn't outlive
// the app. It waits at most shutdownWait for each step. Safe to call more
// than once.
func (c *Controller) Shutdown() {
	c.Cancel()
	deadline := time.Now().Add(shutdownWait)
	for c.Running() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	// what the groups got to, for the next launch's Activity pane
	c.mu.Lock()
	for _, g := range c.groups {
		c.recordGroupLocked(g)
	}
	c.mu.Unlock()
	c.saveHistoryNow()
	done := make(chan struct{})
	go func() {
		browser.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(shutdownWait):
	}
}

// events maps core.Events onto group g's state. Each method takes the
// controller's lock itself; core calls them from several goroutines at once.
type events struct {
	c *Controller
	g *group
}

var _ core.Events = events{}

// advance moves a row into the archiving phase at its first archived step,
// the point the CLI's bar switches its label: the downloads fill the bar up
// to the download share and no further. Callers hold mu.
func advance(j *Job) {
	if j.State == JobDownloading && j.Done > j.Total*core.DownloadWeight/(core.DownloadWeight+core.ArchiveWeight) {
		j.State = JobArchiving
	}
}

func (e events) BundleStart(pages int64) {
	c, g := e.c, e.g
	c.mu.Lock()
	if g.bundle != nil {
		g.bundle.Total = pages * (core.DownloadWeight + core.ArchiveWeight)
		// a corrected total comes again mid-download, so keep the status a chapter set
		if g.bundle.StatusMsg != nil && g.bundle.StatusMsg.Key == "job.phase.gathering" {
			g.bundle.setStatus(msg("job.phase.downloading", nil))
		}
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Fetching(listed grabber.Filterable) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackFetching(c, listed)
	// a bundle or a volume has its own row
	if g.bundle == nil && g.listedVol[listed] == nil {
		j := &Job{Title: grabber.ChapterLabel(listed), Tags: g.releaseTags(listed), Number: listed.GetNumber(), State: JobFetching, Key: chapKey(listed)}
		j.setStatus(msg("job.status.fetching", nil))
		g.jobs = append(g.jobs, j)
		g.fetching[listed] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Fetched(listed grabber.Filterable, ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackFetched(listed, ch)
	if j := g.fetching[listed]; j != nil {
		// the fetched chapter's title can differ from the listing's
		j.Title, j.Tags = grabber.ChapterLabel(ch), g.releaseTags(ch)
		g.byChap[ch] = j
		delete(g.fetching, listed)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) FetchFailed(listed grabber.Filterable, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackFetchFailed(c, listed, err)
	g.stats.failures = append(g.stats.failures, newFailure(g.releaseLabel(listed), err))
	if j := g.fetching[listed]; j != nil {
		j.State = JobFailed
		j.setReasonErr(err)
		j.clearStatus()
		delete(g.fetching, listed)
	} else if g.bundle == nil && g.listedVol[listed] == nil {
		j := &Job{Title: grabber.ChapterLabel(listed), Tags: g.releaseTags(listed), Number: listed.GetNumber(), State: JobFailed, Key: chapKey(listed)}
		j.setReasonErr(err)
		g.jobs = append(g.jobs, j)
	}
	c.appendMsgLocked(LogError, msg("log.errorFetching", map[string]any{"label": g.releaseLabel(listed), "reason": err.Error()}))
	c.mu.Unlock()
	c.emit()
}

func (e events) Skipped(ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackSkipped(c, ch)
	g.stats.skipped++
	if g.bundle == nil && g.chapVol[ch] == nil {
		g.endRow(ch, JobSkipped, msg("job.reason.skipped", nil))
	}
	c.appendMsgLocked(LogWarn, msg("log.skipping", map[string]any{"label": g.releaseLabel(ch)}))
	c.mu.Unlock()
	c.emit()
}

// Exists is a chapter left out because its file is already in the output folder.
// Its row is the one Fetching made, or the fetched chapter's (after the
// re-check), or a new one when the check ran before any fetch.
func (e events) Exists(listed grabber.Filterable, name string) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.stats.existing++
	g.trackExists(c, listed)
	reason := msg("job.reason.exists", map[string]any{"name": name})
	j := g.fetching[listed]
	if ch, ok := listed.(*grabber.Chapter); j == nil && ok {
		j = g.byChap[ch]
	}
	if j != nil {
		j.State = JobSkipped
		j.setReason(reason)
		j.clearStatus()
		delete(g.fetching, listed)
	} else if g.bundle == nil {
		j = &Job{Title: grabber.ChapterLabel(listed), Tags: g.releaseTags(listed), Number: listed.GetNumber(), State: JobSkipped}
		j.setReason(reason)
		g.jobs = append(g.jobs, j)
	}
	c.appendMsgLocked(LogInfo, msg("log.exists", map[string]any{"name": name}))
	c.mu.Unlock()
	c.emit()
}

func (e events) FilenameFailed(ch *grabber.Chapter, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackFilenameFailed(ch)
	reason := msg("job.reason.filename", map[string]any{"reason": err.Error()})
	g.stats.failures = append(g.stats.failures, windowFailure(g.releaseLabel(ch), reason))
	if g.bundle == nil {
		g.endRow(ch, JobFailed, reason)
	}
	c.appendMsgLocked(LogError, msg("log.errorFilename", map[string]any{"label": g.releaseLabel(ch), "reason": err.Error()}))
	c.mu.Unlock()
	c.emit()
}

func (e events) Started(ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackStarted(c, ch)
	if g.bundle != nil {
		g.bundle.setStatus(msg("job.phase.downloadingLabel", map[string]any{"label": g.releaseLabel(ch)}))
	} else if j := g.chapVol[ch]; j != nil {
		// its row is too narrow for the chapter's title: the phase and
		// percentage say enough. The bar moves with VolumeProgress.
		j.clearStatus()
	} else if j := g.byChap[ch]; j != nil {
		// the row Fetching made becomes the downloading one
		j.Total = ch.PagesCount * (core.DownloadWeight + core.ArchiveWeight)
		j.State = JobDownloading
		j.clearStatus()
	} else {
		// the CLI's bar: each page's download and archive steps
		j := &Job{Title: grabber.ChapterLabel(ch), Tags: g.releaseTags(ch), Number: ch.GetNumber(), Total: ch.PagesCount * (core.DownloadWeight + core.ArchiveWeight), State: JobDownloading, Key: g.keyOf(ch)}
		g.jobs = append(g.jobs, j)
		g.byChap[ch] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Page(ch *grabber.Chapter, idx int, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackPage(ch, err)
	if err != nil {
		c.appendMsgLocked(LogError, msg("log.errorPage", map[string]any{"page": idx + 1, "label": g.releaseLabel(ch), "reason": err.Error()}))
	} else if j := g.jobFor(ch); j != nil && !j.Volume && g.live(ch) {
		// a volume's row moves with VolumeProgress instead
		j.Done += core.DownloadWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) DownloadFailed(ch *grabber.Chapter, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.failedC[ch] = true
	g.trackDownloadFailed(c, ch, err)
	g.stats.failures = append(g.stats.failures, newFailure(g.releaseLabel(ch), err))
	// a volume goes on with its other chapters
	if j := g.byChap[ch]; j != nil && g.bundle == nil {
		j.State = JobFailed
		reason := msg("job.reason.download", map[string]any{"reason": err.Error()})
		if m := messageOf(err); m != nil {
			reason = *m
		}
		j.setReason(reason)
		j.clearStatus()
	}
	c.appendMsgLocked(LogError, msg("log.errorDownload", map[string]any{"label": g.releaseLabel(ch), "reason": err.Error()}))
	c.mu.Unlock()
	c.emit()
}

func (e events) Archived(ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackArchived(ch)
	if j := g.byChap[ch]; j != nil && g.bundle == nil && j.State != JobFailed {
		j.Done += core.ArchiveWeight
		advance(j)
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Packed(ch *grabber.Chapter, name string, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackPacked(c, ch, err)
	j := g.byChap[ch]
	if err != nil {
		reason := msg("job.reason.packing", map[string]any{"reason": err.Error()})
		g.stats.failures = append(g.stats.failures, windowFailure(g.releaseLabel(ch), reason))
		if j != nil {
			j.State = JobFailed
			j.setReason(reason)
		}
		c.appendMsgLocked(LogError, msg("log.packFailed", map[string]any{"label": g.releaseLabel(ch), "reason": err.Error()}))
	} else {
		g.stats.saved++
		if j != nil {
			j.State = JobDone
			// the skipped pages were never counted, so the bar is exact only now
			j.Done = j.Total
		}
		c.appendMsgLocked(LogInfo, msg("log.saved", map[string]any{"name": name}))
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) Downloaded(ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	g.trackDownloaded(c, ch)
	// the pages were counted as they came in; this only tallies the chapter
	g.stats.saved++
	c.mu.Unlock()
	c.emit()
}

func (e events) BundlePacking() {
	c, g := e.c, e.g
	c.mu.Lock()
	if g.bundle != nil {
		g.bundle.setStatus(msg("job.phase.creatingBundle", map[string]any{"range": g.rng}))
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) BundleArchived(n int) {
	c, g := e.c, e.g
	c.mu.Lock()
	if g.bundle != nil {
		g.bundle.Done += int64(n) * core.ArchiveWeight
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumeStart(v *core.Volume) {
	c, g := e.c, e.g
	c.mu.Lock()
	j := &Job{Number: v.FirstNumber(), Total: volumeScale, State: JobDownloading, Volume: true}
	j.setTitle(g.volumeTitle(v))
	j.setStatus(msg("job.phase.waiting", nil))
	if g.runinfo.Parts {
		// one chapter's parts share its group and language
		j.Tags = g.releaseTags(v.Chapters[0])
	}
	g.jobs = append(g.jobs, j)
	g.byVolume[v] = j
	for _, chap := range v.Chapters {
		g.listedVol[chap] = j
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumeChapter(v *core.Volume, ch *grabber.Chapter) {
	c, g := e.c, e.g
	c.mu.Lock()
	if j := g.byVolume[v]; j != nil {
		g.chapVol[ch] = j
	}
	c.mu.Unlock()
}

func (e events) VolumePacking(v *core.Volume) {
	c, g := e.c, e.g
	c.mu.Lock()
	if j := g.byVolume[v]; j != nil {
		j.State = JobArchiving
		j.setStatus(msg("job.phase.packing", nil))
	}
	c.mu.Unlock()
	c.emit()
}

// volumeScale is the Total of a volume's row, whose Done is VolumeProgress's
// fraction of it
const volumeScale = 10000

func (e events) VolumeProgress(v *core.Volume, fraction float64) {
	c, g := e.c, e.g
	c.mu.Lock()
	// core never reports a volume going back; max keeps the row honest even
	// so, and leaves a row that already ended alone
	if j := g.byVolume[v]; j != nil && j.State != JobDone && j.State != JobFailed && j.State != JobSkipped {
		j.Done = max(j.Done, int64(fraction*volumeScale))
	}
	c.mu.Unlock()
	c.emit()
}

func (e events) VolumePacked(v *core.Volume, name string, err error) {
	c, g := e.c, e.g
	c.mu.Lock()
	j := g.byVolume[v]
	label := v.Label()
	switch {
	case errors.Is(err, context.Canceled):
		// a cancelled run says so in its summary; the row only stops
		if j != nil {
			j.State = JobSkipped
			j.setReason(msg("job.reason.cancelled", nil))
		}
	case errors.Is(err, core.ErrSkippedVolume):
		// its chapters were each counted as skipped already
		if j != nil {
			j.State = JobSkipped
			j.setReasonErr(err)
		}
	case errors.Is(err, core.ErrExists):
		// its file is already there, so none of its chapters was fetched
		g.stats.existing += len(v.Chapters)
		if j != nil {
			j.State = JobSkipped
			j.setReason(msg("job.reason.exists", map[string]any{"name": name}))
		}
		c.appendMsgLocked(LogInfo, msg("log.exists", map[string]any{"name": name}))
	case err != nil:
		g.stats.failures = append(g.stats.failures, newFailure(label, err))
		if j != nil {
			j.State = JobFailed
			j.setReasonErr(err)
		}
		c.appendMsgLocked(LogError, msg("log.errorPacking", map[string]any{"label": label, "reason": err.Error()}))
	default:
		g.stats.volumes++
		if j != nil {
			j.State = JobDone
			j.Done = j.Total
		}
		c.appendMsgLocked(LogInfo, msg("log.saved", map[string]any{"name": name}))
	}
	if j != nil {
		j.clearStatus()
	}
	g.trackVolumePacked(c, v, err)
	c.mu.Unlock()
	c.emit()
}
