// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// A chapter being downloaded is claimed with a file named after the one it
// will be packed into plus ClaimSuffix, next to it, so two copies of the
// program downloading into one folder (two app windows, or the app and the
// command line) don't both download it: the second waits until the claim is
// gone, then skips the chapter when its file is there (with --skip-existing)
// or downloads it when it isn't (the first copy failed). The claimer touches
// the file every ClaimBeat; one untouched for ClaimStale was left by a copy
// that stopped, and is taken over. The file holds "key: value" lines saying
// which program made it, for a person who finds one.
const ClaimSuffix = ".downloading"

// The claim timings, vars so tests can shrink them
var (
	// ClaimBeat is how often a claim is touched
	ClaimBeat = 30 * time.Second
	// ClaimStale is how long an untouched claim stays in force
	ClaimStale = 2 * time.Minute
	// ClaimPoll is how often a waiting copy looks at the claim again
	ClaimPoll = 2 * time.Second
)

// claim is a held claim file
type claim struct {
	path string
	stop chan struct{}
	done chan struct{}
}

// tryClaim creates the claim file at path. It returns nil, false when another
// copy holds a live claim, and takes over a stale one.
func tryClaim(path string) (*claim, bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	for range 2 {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			host, _ := os.Hostname()
			_, err = fmt.Fprintf(f, "# a copy of Manga Downloader is downloading this file; it's removed when done\nprocess: %d\nhost: %s\nstarted: %s\n",
				os.Getpid(), host, time.Now().Format(time.RFC3339))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				os.Remove(path)
				return nil, false, err
			}
			c := &claim{path: path, stop: make(chan struct{}), done: make(chan struct{})}
			go c.beat()
			return c, true, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, false, err
		}
		fi, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				// released between the two calls
				continue
			}
			return nil, false, err
		}
		if time.Since(fi.ModTime()) < ClaimStale {
			return nil, false, nil
		}
		// left by a copy that stopped
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, false, err
		}
	}
	return nil, false, nil
}

// beat touches the claim every ClaimBeat until release
func (c *claim) beat() {
	defer close(c.done)
	t := time.NewTicker(ClaimBeat)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			now := time.Now()
			os.Chtimes(c.path, now, now)
		}
	}
}

// release removes the claim
func (c *claim) release() {
	if c == nil {
		return
	}
	close(c.stop)
	<-c.done
	os.Remove(c.path)
}

// claimChapter claims the chapter's file name, waiting (and telling ev once)
// while another copy holds it. It returns false when the chapter isn't to be
// downloaded: its file came out while waiting and skipExisting is set (ev is
// told), or ctx was cancelled (err is set). A claim that can't be made at all
// (say a read-only folder) isn't in the way: the chapter goes on without one,
// and its packing reports the folder's problem.
func claimChapter(ctx context.Context, outDir, name string, chapter *grabber.Chapter, skipExisting bool, ev Events) (c *claim, ok bool, err error) {
	path := filepath.Join(outDir, name) + ClaimSuffix
	told := false
	for {
		if told && skipExisting && packer.Exists(outDir, name) {
			ev.Exists(chapter, name)
			return nil, false, nil
		}
		c, got, err := tryClaim(path)
		if err != nil || got {
			return c, true, nil
		}
		if !told {
			ev.Waiting(chapter, name)
			told = true
		}
		t := time.NewTimer(ClaimPoll)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil, false, ctx.Err()
		case <-t.C:
		}
	}
}
