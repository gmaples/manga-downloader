// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A site's parallel level is how many of its requests may be under way at
// once, over every download sharing its pace (see Share). It's learned from
// how fast the site answers, measured in windows of at least Window and of
// enough requests:
//
//   - A site with no saved level starts at StartParallel and doubles while each
//     doubling is at least 25% faster, then steps back to the best level and
//     settles there.
//   - A settled site tries one more every ProbeEvery windows and keeps it when
//     it's at least 5% faster, else goes back.
//   - Trouble (a 429, an overloaded 5xx, a timeout, a dropped connection)
//     halves the level at once and holds off any rise for Hold. Trouble within
//     Window of the last cut doesn't cut again: those requests were sent
//     before it. For the rest of the run the site stays below the level that
//     had trouble, so a touchy site isn't pushed into 429s over and over (each
//     of which raises its gap and its cooldown).
//   - A window counts only when requests were queued for a slot through most
//     of it, so a run's last pages, which can't fill the slots, teach nothing.
//     The first window after a change doesn't count either: it has the
//     requests of the old level ending and those of the new one starting.
//
// The gap in the rate limits file still applies on top: the level never
// sends requests closer together than it. The level is saved, one "site:
// number" line per site, in ParallelFileName next to the rate limits file,
// when the last download sharing the book ends and right after a cut, and a
// site starts the next run from it.

// ParallelFileName is the name of the learned levels' file in the download root
const ParallelFileName = "site-parallel.txt"

// The learning values, vars so tests can shrink them
var (
	// StartParallel is the level a site with no saved one starts at
	StartParallel = 4
	// MaxParallel caps a level: the most chapters (5) times the most pages
	// (10) one download runs at once
	MaxParallel = 50
	// Window is the least time a measuring window lasts
	Window = 3 * time.Second
	// Hold is how long a cut holds off any rise
	Hold = 30 * time.Second
	// ProbeEvery is how many windows a settled site waits between two tries
	// of one more
	ProbeEvery = 5
)

const (
	// doubleGain is how much faster a doubling has to be to keep doubling
	doubleGain = 1.25
	// probeGain is how much faster one more has to be to be kept
	probeGain = 1.05
	// minSamples is the fewest requests a window measures
	minSamples = 8
)

// parallelHeader starts a new learned levels' file
const parallelHeader = `# Manga Downloader learned pace: how many page downloads run at once for
# each site, as "site: number". The downloader learns it from how fast the
# site answers, and saves it here. The times in site-rate-limits.txt still
# cap how fast requests go out. Edit freely: a number here is where the site
# starts next time.
`

// parseParallelLine reads one line of the learned levels' file: ok is false
// for a blank line or a comment
func parseParallelLine(line string) (key string, n int, ok bool, err error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", 0, false, nil
	}
	k, v, found := strings.Cut(line, ":")
	if !found {
		return "", 0, false, fmt.Errorf("%q isn't \"site: number\"", line)
	}
	key = SiteKey("//" + strings.TrimSpace(k))
	if key == "" {
		return "", 0, false, fmt.Errorf("%q has no site", line)
	}
	n, err = strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return "", 0, false, fmt.Errorf("%q: the number should be a whole number from 1 up", line)
	}
	return key, n, true, nil
}

// readParallel reads the learned levels' file in the book's folder
func (b *Book) readParallel() {
	path := filepath.Join(filepath.Dir(b.path), ParallelFileName)
	readLines(path, ParallelFileName, b.warn, func(line string, i int) {
		key, n, ok, err := parseParallelLine(line)
		if err != nil {
			b.warn("%s line %d: %s (ignored)", ParallelFileName, i, err)
			return
		}
		if ok {
			if _, seen := b.pars[key]; !seen {
				b.pars[key] = n
			}
		}
	})
}

// saveParallel writes the levels of the book's sites that changed since they
// were read or last saved
func (b *Book) saveParallel() {
	b.mu.Lock()
	sites := make([]*Site, 0, len(b.sites))
	for _, s := range b.sites {
		sites = append(sites, s)
	}
	b.mu.Unlock()
	for _, s := range sites {
		s.mu.Lock()
		n, saved := s.par, s.savedPar
		s.savedPar = n
		s.mu.Unlock()
		if n != saved {
			b.writeParallel(s.key, n)
		}
	}
}

// writeParallel writes key's level to the learned levels' file
func (b *Book) writeParallel(key string, n int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pars[key] = n
	path := filepath.Join(filepath.Dir(b.path), ParallelFileName)
	match := func(line string) bool {
		k, _, ok, err := parseParallelLine(line)
		return err == nil && ok && k == key
	}
	if err := writeLine(path, parallelHeader, key+": "+strconv.Itoa(n), match); err != nil {
		b.warn("saving %s: %s", ParallelFileName, err)
	}
}

// startParallel sets s's level from the book's saved one, or StartParallel
// with doubling. Called with b.mu held, before s is shared.
func (s *Site) startParallel(saved int, ok bool) {
	s.free = make(chan struct{})
	if ok {
		s.par = min(max(saved, 1), MaxParallel)
		s.savedPar = saved
		return
	}
	// a site's line is written once its level moves from where it started,
	// so a run that learned nothing leaves no file behind
	s.par = min(max(StartParallel, 1), MaxParallel)
	s.savedPar = s.par
	s.doubling = true
}

// Parallel is how many requests of the site may be under way at once now
func (s *Site) Parallel() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.par
}

// acquire blocks until the site has a free slot and takes it
func (s *Site) acquire(ctx context.Context) error {
	for {
		s.mu.Lock()
		if s.inflight < s.par {
			s.inflight++
			if s.winStart.IsZero() {
				s.winStart = time.Now()
			}
			s.mu.Unlock()
			return nil
		}
		s.waiting++
		free := s.free
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.waiting--
			s.mu.Unlock()
			return ctx.Err()
		case <-free:
			s.mu.Lock()
			s.waiting--
			s.mu.Unlock()
		}
	}
}

// wakeLocked wakes the requests queued for a slot. Called with s.mu held.
func (s *Site) wakeLocked() {
	close(s.free)
	s.free = make(chan struct{})
}

// resetWindowLocked starts a new measuring window. Called with s.mu held.
func (s *Site) resetWindowLocked(now time.Time) {
	s.winStart, s.winBytes, s.winDone, s.winBusy = now, 0, 0, 0
}

// Done ends a request Wait let out, freeing its slot. bytes is what it
// downloaded, negative when it's no page (a chapter's page list), which
// frees the slot without counting towards the speed. trouble is whether it
// failed in a way that says the site is overloaded (see the downloader's
// Trouble).
func (s *Site) Done(bytes int64, trouble bool) {
	s.mu.Lock()
	s.inflight--
	busy := s.waiting > 0
	now := time.Now()
	var msg string
	cut := false
	switch {
	case trouble:
		if s.par > 1 && now.Sub(s.lastCut) >= Window {
			old := s.par
			s.ceiling = old
			s.par = max(1, s.par/2)
			s.warm = true
			s.lastCut, s.holdUntil = now, now.Add(Hold)
			s.doubling, s.probing, s.settle, s.prevRate = false, false, ProbeEvery, 0
			s.resetWindowLocked(now)
			msg = fmt.Sprintf("%s is struggling: down from %d to %d downloads at once (saved in %s)", s.key, old, s.par, ParallelFileName)
			cut = true
		} else if s.par == 1 {
			s.ceiling, s.holdUntil = 2, now.Add(Hold)
		}
	case bytes >= 0:
		s.winBytes += bytes
		s.winDone++
		if busy {
			s.winBusy++
		}
		if s.winDone >= max(2*s.par, minSamples) && now.Sub(s.winStart) >= Window {
			msg = s.judgeLocked(now)
		}
	}
	if s.inflight == 0 && s.waiting == 0 {
		// idle: the next window starts with the next request
		s.winStart, s.winBytes, s.winDone, s.winBusy = time.Time{}, 0, 0, 0
	}
	s.wakeLocked()
	s.mu.Unlock()

	if msg != "" {
		s.book.info("%s", msg)
	}
	if cut {
		s.book.saveParallel()
	}
}

// judgeLocked ends a window at now, changing the level from its speed, and
// returns what to report, "" for nothing. Called with s.mu held.
func (s *Site) judgeLocked(now time.Time) string {
	rate := float64(s.winBytes) / now.Sub(s.winStart).Seconds()
	busy := s.winBusy*4 >= s.winDone*3
	s.resetWindowLocked(now)
	if !busy {
		return ""
	}
	if s.warm {
		// the first window at a new level has the requests of the old one
		// ending and the new ones starting: it says nothing about the level
		s.warm = false
		return ""
	}
	before := s.par
	msg := s.decideLocked(rate, now)
	if s.par != before {
		s.warm = true
	}
	return msg
}

// decideLocked changes the level from rate, the speed of a busy window ended
// at now. Called with s.mu held.
func (s *Site) decideLocked(rate float64, now time.Time) string {
	if now.Before(s.holdUntil) {
		s.prevRate, s.prevPar = rate, s.par
		return ""
	}
	switch {
	case s.doubling:
		if s.prevRate == 0 || rate >= s.prevRate*doubleGain {
			s.prevRate, s.prevPar = rate, s.par
			if s.canRiseLocked() {
				s.par = min(2*s.par, s.topLocked())
				return ""
			}
		} else {
			s.par = s.prevPar
		}
		s.doubling, s.settle = false, ProbeEvery
		return fmt.Sprintf("%s settled at %d downloads at once", s.key, s.par)
	case s.probing:
		s.probing = false
		if rate >= s.prevRate*probeGain {
			s.prevRate, s.prevPar = rate, s.par
			if s.canRiseLocked() {
				s.par++
				s.probing = true
			}
			return ""
		}
		s.par, s.settle = s.prevPar, ProbeEvery
		return ""
	default:
		s.prevRate, s.prevPar = rate, s.par
		if s.settle > 0 {
			s.settle--
			return ""
		}
		if s.canRiseLocked() {
			s.par++
			s.probing = true
		}
		return ""
	}
}

// topLocked is the highest level s may rise to: MaxParallel, or below the
// level that had trouble. Called with s.mu held.
func (s *Site) topLocked() int {
	if s.ceiling > 0 {
		return max(1, min(MaxParallel, s.ceiling-1))
	}
	return MaxParallel
}

// canRiseLocked reports whether s's level may rise. Called with s.mu held.
func (s *Site) canRiseLocked() bool {
	return s.par < s.topLocked()
}

// Parallel returns how many requests of the site key may be under way at
// once now, over the books the downloads running in this process share (see
// Share), the most of them, and 0 when no running download uses the site
func Parallel(key string) int {
	sharedMu.Lock()
	books := make([]*Book, 0, len(shared))
	for _, sb := range shared {
		books = append(books, sb.book)
	}
	sharedMu.Unlock()

	n := 0
	for _, b := range books {
		b.mu.Lock()
		s := b.sites[key]
		b.mu.Unlock()
		if s != nil {
			n = max(n, s.Parallel())
		}
	}
	return n
}
