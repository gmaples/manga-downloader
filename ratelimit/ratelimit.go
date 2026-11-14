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

	mu    sync.Mutex
	gaps  map[string]time.Duration
	sites map[string]*Site
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
	b := &Book{path: filepath.Join(dir, FileName), warn: warn, info: info, gaps: map[string]time.Duration{}, sites: map[string]*Site{}}
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
	data, err := os.ReadFile(b.path)
	if err != nil && !os.IsNotExist(err) {
		b.warn("%s: %s", FileName, err)
		return
	}
	text := string(data)
	if text == "" {
		text = header
	}
	entry := key + ": " + Format(gap)
	lines := strings.Split(text, "\n")
	replaced := false
	for i, line := range lines {
		if k, _, ok, err := parseLine(line); err == nil && ok && k == key {
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
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		b.warn("saving %s: %s", FileName, err)
		return
	}
	if err := os.Rename(tmp, b.path); err != nil {
		os.Remove(tmp)
		b.warn("saving %s: %s", FileName, err)
	}
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
// Hit if the request gets a 429. Waiters take up a change at once.
func (s *Site) Wait(ctx context.Context) (uint64, error) {
	for {
		s.mu.Lock()
		now := time.Now()
		slot := now
		if s.until.After(slot) {
			slot = s.until
		}
		listed := s.listed
		if listed && s.next.After(slot) {
			slot = s.next
		}
		if listed {
			s.next = slot.Add(s.gap)
		}
		gen, changed := s.gen, s.changed
		s.mu.Unlock()

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
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()

	s.book.info("%s answered 429 Too Many Requests: waiting %s, then one request every %s (saved in %s)", s.key, Format(cool), Format(gap), FileName)
	s.book.save(s.key, gap)
	return false
}
