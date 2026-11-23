// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/internal/filelock"
)

// Copies of the program running at once (two app windows, or the app and the
// command line) share a site's pace through PaceFileName, next to the rate
// limits file: for each site, when its next request may go out ("next"), the
// end of the pause after a 429 ("until"), and the gap the copies that have
// one last set ("gap"). Every request of a site that has a gap or a pause, in
// any copy, reads and rewrites it under PaceLockName, so the copies' requests
// together keep the gap, and a 429 one copy gets pauses them all. Each copy
// still learns its own parallel level. The file is rewritten as the program
// runs, and can be deleted when no copy is running. PaceLockName is an OS
// file lock (internal/filelock) that the system lets go of when a copy stops;
// the file itself stays in the folder, which is normal.
const (
	PaceFileName = "site-pace.txt"
	PaceLockName = "site-pace.lock"
)

// The shared pace's lock timings, vars so tests can shrink them
var (
	// paceLockWait is how long a copy tries for the lock before going on
	// with its own pace alone
	paceLockWait = 2 * time.Second
	// paceKeep is how long a site's past times stay in the file
	paceKeep = time.Minute
)

const paceHeader = `# Manga Downloader shared pace: copies of the program running at once read
# and rewrite this file before each request to a site with a time or a pause,
# so together they keep its time and its pause after a 429. It's rewritten as
# the program runs; delete it only when no copy is running.
`

// paceEntry is one site's shared pace
type paceEntry struct {
	next, until time.Time
	gap         time.Duration
}

// pacePath is the shared pace file of the book
func (b *Book) pacePath() string {
	return filepath.Join(filepath.Dir(b.path), PaceFileName)
}

// lockPace takes the shared pace's lock, waiting up to paceLockWait. It's an
// OS lock, so a copy that stops while holding it lets go of it.
func lockPace(dir string) (unlock func(), err error) {
	path := filepath.Join(dir, PaceLockName)
	deadline := time.Now().Add(paceLockWait)
	for {
		release, err := filelock.TryLock(path)
		if err == nil {
			return release, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%s is held by another copy: %w", PaceLockName, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// readPace reads the shared pace file; a missing or unreadable one is empty
func readPace(path string) map[string]paceEntry {
	m := map[string]paceEntry{}
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		site, field, ok := strings.Cut(k, " ")
		if !ok {
			continue
		}
		e := m[site]
		v = strings.TrimSpace(v)
		switch field {
		case "next":
			e.next, _ = time.Parse(time.RFC3339Nano, v)
		case "until":
			e.until, _ = time.Parse(time.RFC3339Nano, v)
		case "gap":
			e.gap, _ = time.ParseDuration(v)
		}
		m[site] = e
	}
	return m
}

// writePace writes the shared pace file, leaving out the sites with no gap
// whose times are over
func writePace(path string, m map[string]paceEntry) error {
	cutoff := time.Now().Add(-paceKeep)
	sites := make([]string, 0, len(m))
	for site, e := range m {
		if e.gap <= 0 && e.next.Before(cutoff) && e.until.Before(cutoff) {
			continue
		}
		sites = append(sites, site)
	}
	sort.Strings(sites)
	var sb strings.Builder
	sb.WriteString(paceHeader)
	for _, site := range sites {
		e := m[site]
		if !e.next.IsZero() {
			fmt.Fprintf(&sb, "%s next: %s\n", site, e.next.UTC().Format(time.RFC3339Nano))
		}
		if !e.until.IsZero() {
			fmt.Fprintf(&sb, "%s until: %s\n", site, e.until.UTC().Format(time.RFC3339Nano))
		}
		if e.gap > 0 {
			fmt.Fprintf(&sb, "%s gap: %s\n", site, Format(e.gap))
		}
	}
	return replaceFile(path, []byte(sb.String()))
}

// sharedSlot moves a request of key, which this copy would send at slot with
// its gap and its pause ending at until, to the shared pace: it returns when
// the request may go out over every copy, and the gap that applies after it,
// the larger of this copy's and the shared one, and when the shared pause
// ends (zero for none), which another copy's 429 may have set. Nothing is read or written
// for a site with no gap and no pause, unless the file has one for it. A file
// that can't be locked leaves this copy's own pace.
func (b *Book) sharedSlot(key string, slot time.Time, gap time.Duration, until time.Time) (time.Time, time.Duration, time.Time) {
	path := b.pacePath()
	if gap <= 0 && !until.After(slot) {
		if _, err := os.Stat(path); err != nil {
			return slot, gap, time.Time{}
		}
	}
	unlock, err := lockPace(filepath.Dir(path))
	if err != nil {
		return slot, gap, time.Time{}
	}
	defer unlock()
	m := readPace(path)
	e, had := m[key]
	if !had && gap <= 0 && !until.After(slot) {
		return slot, gap, time.Time{}
	}
	if e.gap == 0 && gap > 0 {
		e.gap = gap
	}
	g := max(gap, e.gap)
	for _, t := range []time.Time{e.next, e.until} {
		if t.After(slot) {
			slot = t
		}
	}
	if until.After(e.until) {
		e.until = until
	}
	e.next = slot.Add(g)
	m[key] = e
	if err := writePace(path, m); err != nil {
		b.paceWarn.Do(func() { b.warn("saving %s: %s", PaceFileName, err) })
	}
	return slot, g, e.until
}

// sharedChange tells the other copies about a change of key's gap (a 429's
// rise, or a calm's fall; 0 when the site's line was taken out) and, after a
// 429, of its pause, which ends at until (zero for none): the next request
// of any copy then goes when it ends
func (b *Book) sharedChange(key string, gap time.Duration, until time.Time) {
	path := b.pacePath()
	unlock, err := lockPace(filepath.Dir(path))
	if err != nil {
		return
	}
	defer unlock()
	m := readPace(path)
	e := m[key]
	e.gap = gap
	// as in one copy (see Hit), the next request goes when the pause ends
	if !until.IsZero() {
		if until.After(e.until) {
			e.until = until
		}
		e.next = until
	}
	m[key] = e
	if err := writePace(path, m); err != nil {
		b.paceWarn.Do(func() { b.warn("saving %s: %s", PaceFileName, err) })
	}
}

// sharedPause is when key's shared pause ends, zero for none: a 429 that
// another copy got sets it
func (b *Book) sharedPause(key string) time.Time {
	path := b.pacePath()
	if _, err := os.Stat(path); err != nil {
		return time.Time{}
	}
	unlock, err := lockPace(filepath.Dir(path))
	if err != nil {
		return time.Time{}
	}
	defer unlock()
	return readPace(path)[key].until
}
