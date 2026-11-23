// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// shrink sets the pacing values for a test, restoring them afterwards
func shrink(t *testing.T, startGap, maxGap, startWait, maxWait time.Duration) {
	t.Helper()
	g, mg, w, mw := StartGap, MaxGap, StartWait, MaxWait
	StartGap, MaxGap, StartWait, MaxWait = startGap, maxGap, startWait, maxWait
	t.Cleanup(func() { StartGap, MaxGap, StartWait, MaxWait = g, mg, w, mw })
}

// lines records what a Book reports
type lines struct {
	mu  sync.Mutex
	got []string
}

func (l *lines) add(format string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, fmt.Sprintf(format, a...))
}

func read(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSiteKey(t *testing.T) {
	for in, want := range map[string]string{
		"https://mangalib.org/ru/manga/55134--x": "mangalib.org",
		"https://www.MangaDex.org/title/x":       "mangadex.org",
		"http://127.0.0.1:8080/x":                "127.0.0.1",
		"//comix.to":                             "comix.to",
		"not a url":                              "",
	} {
		if got := SiteKey(in); got != want {
			t.Errorf("SiteKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormat(t *testing.T) {
	for d, want := range map[time.Duration]string{
		time.Second:                "1s",
		1500 * time.Millisecond:    "1.5s",
		3375 * time.Millisecond:    "3.375s",
		15 * time.Second:           "15s",
		time.Minute:                "1m",
		90 * time.Second:           "1m30s",
		5 * time.Minute:            "5m",
		1234567 * time.Microsecond: "1.235s",
	} {
		if got := Format(d); got != want {
			t.Errorf("Format(%v) = %q, want %q", d, got, want)
		}
	}
}

// the file is read with comments, blank lines, CRLF and odd spellings, and a
// line that can't be read is reported and skipped
func TestOpenReadsTheFile(t *testing.T) {
	dir := t.TempDir()
	text := "# comment\r\n\r\nMangaLib.org: 2s\r\nwww.mangadex.org : 1m30s\r\nbroken line\r\nexample.com: soon\r\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	warn := &lines{}
	b := Open(dir, warn.add, nil)
	if b.Site("mangalib.org").gap != 2*time.Second || !b.Site("mangalib.org").Listed() {
		t.Errorf("mangalib.org = %v", b.Site("mangalib.org").gap)
	}
	if b.Site("mangadex.org").gap != 90*time.Second {
		t.Errorf("mangadex.org = %v", b.Site("mangadex.org").gap)
	}
	if b.Site("example.com").Listed() {
		t.Error("example.com was read from a line with a bad time")
	}
	if len(warn.got) != 2 || !strings.Contains(warn.got[0], "line 5") || !strings.Contains(warn.got[1], "line 6") {
		t.Errorf("warnings = %q, want lines 5 and 6", warn.got)
	}
}

// a site listed twice is read from its first line, the one a save
// rewrites, so a time saved is the time read next; the second line is warned about
func TestDuplicateSiteLineKeepsTheFirst(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("a.com: 1s\na.com: 5s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warn := &lines{}
	b := Open(dir, warn.add, nil)
	if got := b.Site("a.com").gap; got != time.Second {
		t.Errorf("read %v, want the first line's 1s", got)
	}
	if len(warn.got) != 1 || !strings.Contains(warn.got[0], "line 2") || !strings.Contains(warn.got[0], "a.com is listed again") {
		t.Errorf("warnings = %q, want one for line 2", warn.got)
	}
	b.save("a.com", 2*time.Second)
	if got := Open(dir, nil, nil).Site("a.com").gap; got != 2*time.Second {
		t.Errorf("after saving 2s, read %v", got)
	}
}

// a missing file is empty, and isn't written until a 429
func TestOpenMissingFile(t *testing.T) {
	dir := t.TempDir()
	b := Open(dir, nil, nil)
	if b.Site("mangalib.org").Listed() {
		t.Error("a site is listed from no file")
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Errorf("the file was made before any 429: %v", err)
	}
}

// a 429 lists the site at StartGap and cools down StartWait; each fresh one
// multiplies both by Growth; the 429s of requests sent before the change don't
// count again; each change is saved with the header on a new file
func TestHitRaisesTheGapAndTheWait(t *testing.T) {
	shrink(t, time.Second, 5*time.Minute, 15*time.Second, 5*time.Minute)
	dir := t.TempDir()
	info := &lines{}
	s := Open(dir, nil, info.add).Site("mangalib.org")

	if s.Hit(0, 0) {
		t.Fatal("gave up on the first 429")
	}
	if s.gap != time.Second || s.wait != 15*time.Second || s.gen != 1 {
		t.Fatalf("after the first 429: gap %v wait %v gen %d", s.gap, s.wait, s.gen)
	}
	if got := read(t, dir); got != header+"mangalib.org: 1s\n" {
		t.Errorf("file =\n%s", got)
	}
	// another request of the same wave: nothing changes
	if s.Hit(0, 0) || s.gen != 1 || s.gap != time.Second {
		t.Errorf("a stale 429 changed things: gen %d gap %v", s.gen, s.gap)
	}
	s.Hit(1, 0)
	if s.gap != 1500*time.Millisecond || s.wait != 22500*time.Millisecond {
		t.Errorf("after the second 429: gap %v wait %v", s.gap, s.wait)
	}
	if got := read(t, dir); got != header+"mangalib.org: 1.5s\n" {
		t.Errorf("file =\n%s", got)
	}
	if len(info.got) != 2 || !strings.Contains(info.got[0], "mangalib.org answered 429") || !strings.Contains(info.got[0], "waiting 15s, then one request every 1s") {
		t.Errorf("info = %q", info.got)
	}
}

// a site listed in the file grows from its own gap
func TestHitGrowsTheListedGap(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("mangalib.org: 4s\n"), 0o644)
	s := Open(dir, nil, nil).Site("mangalib.org")
	s.Hit(0, 0)
	if s.gap != 6*time.Second {
		t.Errorf("gap %v, want 6s", s.gap)
	}
}

// the gap and the wait stop at their caps, and a 429 when the wait is at
// MaxWait gives up
func TestHitCapsAndGivesUp(t *testing.T) {
	shrink(t, time.Second, 2*time.Second, 15*time.Second, 30*time.Second)
	s := Open(t.TempDir(), nil, nil).Site("x.org")
	var gen uint64
	for i := 0; ; i++ {
		if s.Hit(gen, 0) {
			break
		}
		gen = s.gen
		if i > 10 {
			t.Fatal("never gave up")
		}
	}
	// 15s, 22.5s, 30s (capped), then the give up
	if s.gen != 3 || s.wait != 30*time.Second || s.gap != 2*time.Second {
		t.Errorf("gave up at gen %d wait %v gap %v, want 3, 30s, 2s", s.gen, s.wait, s.gap)
	}
}

// saving rewrites only the site's line, keeping comments, other sites, the
// line endings and edits made since the file was read; a new site is appended
func TestSaveKeepsTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	os.WriteFile(path, []byte("# mine\r\nfoo.com: 3s\r\nmangalib.org: 2s\r\n# end\r\n"), 0o644)
	b := Open(dir, nil, nil)
	// edited by hand during the run
	os.WriteFile(path, []byte("# mine\r\nfoo.com: 9s\r\nmangalib.org: 2s\r\n# end\r\n"), 0o644)

	b.Site("mangalib.org").Hit(0, 0)
	if got, want := read(t, dir), "# mine\r\nfoo.com: 9s\r\nmangalib.org: 3s\r\n# end\r\n"; got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
	b.Site("bar.net").Hit(0, 0)
	if got, want := read(t, dir), "# mine\r\nfoo.com: 9s\r\nmangalib.org: 3s\r\n# end\r\nbar.net: 1s\n"; got != want {
		t.Errorf("file =\n%q\nwant\n%q", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("the temporary file was left behind")
	}
}

// a listed site's requests go out at least its gap apart
func TestWaitPaces(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("x.org: 40ms\n"), 0o644)
	s := Open(dir, nil, nil).Site("x.org")
	start := time.Now()
	for range 4 {
		if _, err := s.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
		s.Done(0, false)
	}
	if elapsed := time.Since(start); elapsed < 120*time.Millisecond {
		t.Errorf("4 requests took %v, want at least 3 gaps of 40ms", elapsed)
	}
}

// an unlisted site isn't paced
func TestWaitUnlisted(t *testing.T) {
	s := Open(t.TempDir(), nil, nil).Site("x.org")
	start := time.Now()
	for range 100 {
		s.Wait(context.Background())
		s.Done(0, false)
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("100 unlisted requests took %v", elapsed)
	}
}

// a 429 holds every request for the cooldown, and the Retry-After when longer,
// but never past MaxWait
func TestWaitSitsOutTheCooldown(t *testing.T) {
	shrink(t, time.Millisecond, time.Second, 60*time.Millisecond, 150*time.Millisecond)
	s := Open(t.TempDir(), nil, nil).Site("x.org")
	s.Hit(0, 0)
	start := time.Now()
	s.Wait(context.Background())
	s.Done(0, false)
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Errorf("waited %v after a 429, want the 60ms cooldown", elapsed)
	}
	s.Hit(s.gen, time.Hour)
	start = time.Now()
	s.Wait(context.Background())
	s.Done(0, false)
	if elapsed := time.Since(start); elapsed < 140*time.Millisecond || elapsed > time.Second {
		t.Errorf("waited %v after an hour's Retry-After, want MaxWait (150ms)", elapsed)
	}
}

// requests already waiting take up a change at once: a long gap is cut short
// by a 429 whose cooldown ends sooner, and a waiter can be cancelled
func TestWaitersTakeUpAChange(t *testing.T) {
	shrink(t, time.Millisecond, time.Hour, 20*time.Millisecond, time.Second)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, FileName), []byte("x.org: 10s\n"), 0o644)
	s := Open(dir, nil, nil).Site("x.org")
	s.Wait(context.Background()) // the first goes at once, the next is 10s away
	s.Done(0, false)

	done := make(chan time.Duration)
	start := time.Now()
	go func() {
		s.Wait(context.Background())
		s.Done(0, false)
		done <- time.Since(start)
	}()
	time.Sleep(10 * time.Millisecond)
	s.Hit(s.gen, 0)
	select {
	case d := <-done:
		if d < 20*time.Millisecond {
			t.Errorf("the waiter went after %v, before the cooldown", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the waiter kept its old 10s slot")
	}

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	if _, err := s.Wait(ctx); err != context.Canceled {
		t.Errorf("cancelled wait = %v", err)
	}
}

// a file that can't be saved is reported and the run goes on
func TestSaveFailureIsReported(t *testing.T) {
	warn := &lines{}
	s := Open(filepath.Join(t.TempDir(), "missing"), warn.add, nil).Site("x.org")
	if s.Hit(0, 0) {
		t.Fatal("gave up")
	}
	if len(warn.got) != 1 || !strings.Contains(warn.got[0], "saving "+FileName) {
		t.Errorf("warnings = %q", warn.got)
	}
	if !s.Listed() {
		t.Error("the gap was lost with the file")
	}
}

// Two downloads using one folder at once share its book, so a 429 one of them
// gets paces the other; once both are done, the file is read again.
func TestShareWhileInUse(t *testing.T) {
	shrink(t, time.Second, time.Minute, time.Millisecond, time.Minute)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("example.com: 2s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, releaseA := Share(dir, nil, nil)
	b, releaseB := Share(filepath.Join(dir, "."), nil, nil)
	if a != b {
		t.Fatal("two users of one folder got different books")
	}
	sa, sb := a.Site("example.com"), b.Site("example.com")
	if sa != sb {
		t.Fatal("two users of one folder got different paces for a site")
	}
	sa.Hit(0, 0)
	if got := sb.gapNow(); got != 3*time.Second {
		t.Fatalf("the other user's gap is %s, want 3s", got)
	}

	// a hand edit while the book is in use isn't read yet
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("example.com: 7s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	releaseA()
	releaseA() // a second release changes nothing
	c, releaseC := Share(dir, nil, nil)
	if c != a {
		t.Fatal("a book still in use was opened again")
	}
	releaseC()
	releaseB()

	d, releaseD := Share(dir, nil, nil)
	defer releaseD()
	if d == a {
		t.Fatal("the book was kept after its last user released it")
	}
	if got := d.Site("example.com").gapNow(); got != 7*time.Second {
		t.Fatalf("gap after reopening is %s, want the edited 7s", got)
	}
}

// Cooling reports a site's cooldown in the books in use, and nothing once it's
// over, for another site or after the book was released
func TestCooling(t *testing.T) {
	shrink(t, time.Second, time.Minute, 200*time.Millisecond, time.Minute)
	b, release := Share(t.TempDir(), nil, nil)
	if got := Cooling("example.com"); !got.IsZero() {
		t.Fatalf("Cooling before any 429 = %v, want zero", got)
	}
	b.Site("example.com").Hit(0, 0)
	got := Cooling("example.com")
	if left := time.Until(got); left <= 0 || left > 200*time.Millisecond {
		t.Fatalf("Cooling after a 429 ends in %s, want within the 200ms cooldown", left)
	}
	if other := Cooling("other.example"); !other.IsZero() {
		t.Errorf("Cooling of another site = %v, want zero", other)
	}
	time.Sleep(time.Until(got) + 10*time.Millisecond)
	if after := Cooling("example.com"); !after.IsZero() {
		t.Errorf("Cooling once the cooldown is over = %v, want zero", after)
	}
	b.Site("example.com").Hit(1, 0)
	release()
	if gone := Cooling("example.com"); !gone.IsZero() {
		t.Errorf("Cooling after the release = %v, want zero", gone)
	}
}

// Downloads into different folders don't share
func TestShareByFolder(t *testing.T) {
	a, releaseA := Share(t.TempDir(), nil, nil)
	defer releaseA()
	b, releaseB := Share(t.TempDir(), nil, nil)
	defer releaseB()
	if a == b {
		t.Fatal("two folders share one book")
	}
}

// gapNow is the site's gap, read under its lock
func (s *Site) gapNow() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gap
}
