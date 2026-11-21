// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package ratelimit paces the requests of a download per site, from a plain
// text file kept in the download root (FileName), and slows a site down when
// it answers 429 Too Many Requests.
//
// The file holds one "site: gap" line per site, the gap being the least time
// between two requests for that site's downloads, in Go duration units
// ("500ms", "15s", "1m30s"). Lines starting with # are comments. It's meant
// to be edited by hand, and to travel with the download root (an SD card
// moved to another device takes its limits along).
//
// On a 429 the site's gap is raised (StartGap for a site the file doesn't
// list, else times Growth, up to MaxGap) and saved at once, and every request
// of the site waits a cooldown before going on: StartWait after the run's
// first 429 for the site, times Growth after each further one, up to MaxWait.
// A 429 that comes when the cooldown is already at MaxWait gives up.
//
// A site that goes CalmFor without a 429 while it's in use gets its gap
// divided by Growth, saved at once, and again after each further CalmFor.
// Once the gap would drop below StartGap its line is taken out of the file,
// so a few 429s don't slow the site down for good.
//
// Beside the gap, each site learns how many of its requests to have under way
// at once from how fast it answers, saved in ParallelFileName (see
// parallel.go).
package ratelimit

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileName is the name of the file in the download root
const FileName = "site-rate-limits.txt"

// The pacing values, vars so tests can shrink them
var (
	// StartGap is the gap a site the file doesn't list gets on its first 429
	StartGap = time.Second
	// MaxGap caps a site's gap
	MaxGap = 5 * time.Minute
	// StartWait is the cooldown after a site's first 429 in a run
	StartWait = 15 * time.Second
	// MaxWait caps the cooldown; a 429 at this cooldown gives up
	MaxWait = 5 * time.Minute
	// CalmFor is how long a site goes without a 429 before its gap is
	// divided by Growth
	CalmFor = 5 * time.Minute
)

// Growth is what a gap or a cooldown is multiplied by on each 429
const Growth = 1.5

// header starts a new file
const header = `# Manga Downloader site rate limits: the least time between two requests
# to a site, as "site: time". Units: ms, s, m (500ms, 15s, 1m30s).
# Edit freely. The downloader raises a site's time when it answers 429.
`

// SiteKey is the name a site goes by in the file: the host of its address,
// lower case and without "www.". It's "" when rawURL has no host.
func SiteKey(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

// Book is the file of one download root, read once, with the sites of a run
type Book struct {
	path string
	// warn reports a line that can't be read and a file that can't be saved
	warn func(format string, a ...any)
	// info reports a 429 and what it changed
	info func(format string, a ...any)

	// paceWarn reports a shared pace file that can't be saved once, not at
	// every request
	paceWarn sync.Once

	mu    sync.Mutex
	gaps  map[string]time.Duration
	pars  map[string]int
	sites map[string]*Site
	// mirrors are the other hosts of site-mirrors.txt, by host (see Mirrors)
	mirrors map[string][]string
}

// Open reads the file in dir. A missing file is an empty book, which is only
// written on the first 429. warn and info may be nil.
func Open(dir string, warn, info func(format string, a ...any)) *Book {
	if warn == nil {
		warn = func(string, ...any) {}
	}
	if info == nil {
		info = func(string, ...any) {}
	}
	b := &Book{path: filepath.Join(dir, FileName), warn: warn, info: info, gaps: map[string]time.Duration{}, pars: map[string]int{}, sites: map[string]*Site{}, mirrors: map[string][]string{}}
	b.readParallel()
	b.readMirrors()
	data, err := os.ReadFile(b.path)
	if err != nil {
		if !os.IsNotExist(err) {
			warn("%s: %s", FileName, err)
		}
		return b
	}
	for i, line := range strings.Split(string(data), "\n") {
		key, gap, ok, err := parseLine(line)
		if err != nil {
			warn("%s line %d: %s (ignored)", FileName, i+1, err)
			continue
		}
		if ok {
			b.gaps[key] = gap
		}
	}
	return b
}

// shared holds the books downloads are using, by folder, so the downloads
// running at once in one process (the app's groups) share one pace per site
var (
	sharedMu sync.Mutex
	shared   = map[string]*sharedBook{}
)

// sharedBook is a book and how many downloads use it
type sharedBook struct {
	book  *Book
	users int
}

// Share returns the book of dir that every download running in this process
// shares, opening it (reading the file) when no download uses it yet. release
// ends this use. Once the last user releases, the next Share reads the file
// again, so an edit made between downloads is taken up. The last release
// saves the learned levels (see Parallel). release is safe to call more than
// once.
func Share(dir string, warn, info func(format string, a ...any)) (b *Book, release func()) {
	key := filepath.Clean(dir)
	if abs, err := filepath.Abs(key); err == nil {
		key = abs
	}
	sharedMu.Lock()
	defer sharedMu.Unlock()
	sb := shared[key]
	if sb == nil {
		sb = &sharedBook{book: Open(dir, warn, info)}
		shared[key] = sb
	}
	sb.users++
	var once sync.Once
	return sb.book, func() {
		once.Do(func() {
			sharedMu.Lock()
			sb.users--
			last := sb.users == 0
			if last && shared[key] == sb {
				delete(shared, key)
			}
			sharedMu.Unlock()
			if last {
				sb.book.saveParallel()
			}
		})
	}
}

// Cooling returns when the cooldown after a 429 of the site key ends, over
// the books the downloads running in this process share (see Share), and the
// zero time when it isn't cooling down in any of them
func Cooling(key string) time.Time {
	sharedMu.Lock()
	books := make([]*Book, 0, len(shared))
	for _, sb := range shared {
		books = append(books, sb.book)
	}
	sharedMu.Unlock()

	var until time.Time
	now := time.Now()
	for _, b := range books {
		b.mu.Lock()
		s := b.sites[key]
		b.mu.Unlock()
		if s == nil {
			continue
		}
		s.mu.Lock()
		if s.until.After(now) && s.until.After(until) {
			until = s.until
		}
		s.mu.Unlock()
	}
	return until
}

// parseLine reads one line: ok is false for a blank line or a comment
func parseLine(line string) (key string, gap time.Duration, ok bool, err error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", 0, false, nil
	}
	k, v, found := strings.Cut(line, ":")
	if !found {
		return "", 0, false, fmt.Errorf("%q isn't \"site: time\"", line)
	}
	key = SiteKey("//" + strings.TrimSpace(k))
	if key == "" {
		return "", 0, false, fmt.Errorf("%q has no site", line)
	}
	gap, err = time.ParseDuration(strings.TrimSpace(v))
	if err != nil {
		return "", 0, false, fmt.Errorf("%q: the time should look like 500ms, 15s or 1m30s", line)
	}
	if gap < 0 {
		return "", 0, false, fmt.Errorf("%q: the time can't be negative", line)
	}
	return key, gap, true, nil
}

// Format writes d as the file does: whole milliseconds, "1m" rather than "1m0s"
func Format(d time.Duration) string {
	s := d.Round(time.Millisecond).String()
	if strings.Contains(s, "m") && strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	return s
}

// Site returns the pacing of the site key
func (b *Book) Site(key string) *Site {
	b.mu.Lock()
	defer b.mu.Unlock()
	if s, ok := b.sites[key]; ok {
		return s
	}
	s := &Site{book: b, key: key, changed: make(chan struct{})}
	s.gap, s.listed = b.gaps[key]
	n, ok := b.pars[key]
	s.startParallel(n, ok)
	b.sites[key] = s
	return s
}

// save writes key's gap to the file, rewriting only its line (the first one
// naming it) or appending one, and keeping every other line as it is now on
// disk, so edits made during the run aren't lost
func (b *Book) save(key string, gap time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.gaps[key] = gap
	match := func(line string) bool {
		k, _, ok, err := parseLine(line)
		return err == nil && ok && k == key
	}
	if err := writeLine(b.path, header, key+": "+Format(gap), match); err != nil {
		b.warn("saving %s: %s", FileName, err)
	}
}

// drop takes key's line out of the file (the first one naming it), keeping
// every other line as it is now on disk
func (b *Book) drop(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.gaps, key)
	match := func(line string) bool {
		k, _, ok, err := parseLine(line)
		return err == nil && ok && k == key
	}
	if err := removeLine(b.path, match); err != nil {
		b.warn("saving %s: %s", FileName, err)
	}
}

// removeLine takes the first line match accepts out of the file at path,
// keeping every other line as it is now on disk. A missing file, or one
// without such a line, is left alone.
func removeLine(path string, match func(line string) bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if match(line) {
			lines = append(lines[:i], lines[i+1:]...)
			return replaceFile(path, []byte(strings.Join(lines, "\n")))
		}
	}
	return nil
}

// writeLine writes entry into the file at path in place of the first line
// match accepts, or appends it, keeping every other line as it is now on
// disk, so edits made during the run aren't lost. A new file starts with
// header.
func writeLine(path, header, entry string, match func(line string) bool) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	text := string(data)
	if text == "" {
		text = header
	}
	lines := strings.Split(text, "\n")
	replaced := false
	for i, line := range lines {
		if match(line) {
			lines[i] = entry
			if strings.HasSuffix(line, "\r") {
				lines[i] += "\r"
			}
			replaced = true
			break
		}
	}
	if !replaced {
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		lines = append(lines, entry, "")
	}
	return replaceFile(path, []byte(strings.Join(lines, "\n")))
}

// replaceFile writes data to path through a temporary file and a rename, so
// the file is never left half written
func replaceFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// Site paces the requests of one site. It's safe for concurrent use.
type Site struct {
	book *Book
	key  string

	mu     sync.Mutex
	listed bool
	gap    time.Duration
	// next is the earliest time the next request may go out
	next time.Time
	// until is the end of the cooldown after a 429
	until time.Time
	// wait is the cooldown the next 429 starts from, 0 before any
	wait time.Duration
	// gen counts the 429s that changed the pacing; a request is told its
	// generation, so the 429s of requests sent before a change don't count again
	gen uint64
	// changed is closed and replaced on each change, waking the waiters
	changed chan struct{}
	// calm is when the site's last 429, or the last lowering of its gap, came;
	// the zero time before the site's first request
	calm time.Time

	// the learned level, see parallel.go: par requests may be under way at
	// once, inflight are, waiting are queued for a slot, and free is closed
	// and replaced when a slot frees or par changes
	par, inflight, waiting int
	free                   chan struct{}
	// savedPar is the level in the file, or the level it started at
	savedPar int
	// doubling while the level hasn't settled yet; probing while one more is
	// being tried; settle counts the windows before the next try
	doubling, probing bool
	settle            int
	// prevRate is the speed (bytes a second) of the last window at prevPar
	prevRate float64
	prevPar  int
	// the measuring window: since winStart, winDone requests downloaded
	// winBytes, winBusy of them with requests queued for a slot
	winStart           time.Time
	winBytes           int64
	winDone, winBusy   int
	lastCut, holdUntil time.Time
	// warm is set by a change of level: the next window is a warm-up
	warm bool
	// ceiling is the level that last had trouble, 0 before any: the level
	// stays below it for the rest of the run
	ceiling int
}

// Key is the site's name in the file
func (s *Site) Key() string { return s.key }

// Listed reports whether the site has a gap, from the file or from a 429. A
// site's own built-in pacing applies only while it hasn't: the file wins.
func (s *Site) Listed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listed
}

// Wait blocks until a request may go out, and returns its generation, to give
// Hit if the request gets a 429. Waiters take up a change at once. A request
// Wait lets out holds one of the site's slots (see Parallel) until Done.
func (s *Site) Wait(ctx context.Context) (uint64, error) {
	if err := s.acquire(ctx); err != nil {
		return 0, err
	}
	gen, err := s.paced(ctx)
	if err != nil {
		s.Done(-1, false)
	}
	return gen, err
}

// paced is Wait without the slot: it sits out the gap and the cooldown
func (s *Site) paced(ctx context.Context) (uint64, error) {
	for {
		s.mu.Lock()
		now := time.Now()
		slot := now
		if s.until.After(slot) {
			slot = s.until
		}
		lowered, cleared := s.calmDownLocked(now)
		listed := s.listed
		if listed && s.next.After(slot) {
			slot = s.next
		}
		var gap time.Duration
		if listed {
			gap = s.gap
			s.next = slot.Add(gap)
		}
		until := s.until
		gen, changed := s.gen, s.changed
		s.mu.Unlock()

		switch {
		case cleared:
			s.book.info("%s: no 429 for %s, so its time is taken out of %s", s.key, Format(CalmFor), FileName)
			s.book.drop(s.key)
		case lowered > 0:
			s.book.info("%s: no 429 for %s, now one request every %s (saved in %s)", s.key, Format(CalmFor), Format(lowered), FileName)
			s.book.save(s.key, lowered)
		}
		if cleared || lowered > 0 {
			s.book.sharedChange(s.key, lowered, time.Time{})
		}

		// the other copies of the program running at once may have sent a
		// request later than this one would go, or be pausing after a 429
		shared, g, pause := s.book.sharedSlot(s.key, slot, gap, until)
		s.mu.Lock()
		if shared.After(slot) {
			if n := shared.Add(g); n.After(s.next) {
				s.next = n
			}
		}
		// another copy's 429 restarts this copy's calm too, so it doesn't
		// lower the time that copy just raised
		if pause.After(s.calm) {
			s.calm = pause
		}
		s.mu.Unlock()
		if shared.After(slot) {
			slot = shared
		}

		if d := time.Until(slot); d > 0 {
			t := time.NewTimer(d)
			select {
			case <-ctx.Done():
				t.Stop()
				return gen, ctx.Err()
			case <-changed:
				t.Stop()
				continue
			case <-t.C:
			}
		}
		return gen, nil
	}
}

// calmDownLocked divides the gap by Growth when the site has gone CalmFor
// without a 429, and returns the new gap, or cleared when it would drop below
// StartGap and the site is no longer listed. The calm starts at the site's
// first request. s.mu is held.
func (s *Site) calmDownLocked(now time.Time) (lowered time.Duration, cleared bool) {
	if s.calm.IsZero() {
		s.calm = now
		return 0, false
	}
	if !s.listed || s.gap <= 0 || now.Sub(s.calm) < CalmFor {
		return 0, false
	}
	// another copy's 429 since then restarts the calm (see sharedSlot)
	if pause := s.book.sharedPause(s.key); pause.After(s.calm) {
		s.calm = pause
		return 0, false
	}
	s.calm = now
	gap := time.Duration(float64(s.gap) / Growth)
	if gap < StartGap {
		s.gap, s.listed = 0, false
		return 0, true
	}
	s.gap = gap
	return gap, false
}

// Hit records a 429 to a request of generation gen. retryAfter is what the
// server asked for, 0 if nothing. It raises the gap and the cooldown, unless
// another 429 already did since that request was sent, and reports whether
// to give up: the cooldown was at MaxWait already.
func (s *Site) Hit(gen uint64, retryAfter time.Duration) (giveUp bool) {
	s.mu.Lock()
	if gen != s.gen {
		s.mu.Unlock()
		return false
	}
	if s.wait >= MaxWait {
		s.mu.Unlock()
		return true
	}
	s.gen++
	s.calm = time.Now()
	if !s.listed || s.gap <= 0 {
		s.gap = max(StartGap, s.gap)
		s.listed = true
	} else {
		s.gap = min(time.Duration(float64(s.gap)*Growth), MaxGap)
	}
	if s.wait == 0 {
		s.wait = StartWait
	} else {
		s.wait = min(time.Duration(float64(s.wait)*Growth), MaxWait)
	}
	// the server's Retry-After is honoured, within MaxWait
	cool := min(max(s.wait, retryAfter), MaxWait)
	now := time.Now()
	s.until = now.Add(cool)
	s.next = s.until
	gap := s.gap
	// the other copies are told before this copy's waiters wake, or a waiter
	// could read the shared pace from before the 429
	s.book.sharedChange(s.key, gap, s.until)
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()

	s.book.info("%s answered 429 Too Many Requests: waiting %s, then one request every %s (saved in %s)", s.key, Format(cool), Format(gap), FileName)
	s.book.save(s.key, gap)
	return false
}
