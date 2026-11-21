// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// The marks of the chapter list: what became of each chapter of the loaded
// series, shown beside its checkbox.
const (
	// MarkNone is a chapter with no file in the folder and no download
	MarkNone = ""
	// MarkDone is a chapter saved this session or whose file is in the folder
	MarkDone = "done"
	// MarkDownloading is a chapter of a running download that has begun
	MarkDownloading = "downloading"
	// MarkQueued is a chapter of a running download waiting for its turn
	MarkQueued = "queued"
	// MarkFailed is a chapter whose download failed
	MarkFailed = "failed"
	// MarkCancelled is a chapter left unsaved by a cancelled download
	MarkCancelled = "cancelled"
)

// ChapterMarks returns the mark of each chapter of the loaded list, by index;
// nil when nothing is loaded. The download that took a chapter last says
// what became of it this session; a chapter none took, or that a cancel left
// alone, is done when its file is in the folder (the name Skip existing
// looks for). Restored groups of the last session don't count: their
// chapters were never matched to this listing.
func (c *Controller) ChapterMarks() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil || len(c.series.Chapters) == 0 {
		return nil
	}
	disk := c.diskMarksLocked()
	marks := make([]string, len(c.series.Chapters))
	for i, chap := range c.series.Chapters {
		k := chapKey(chap)
		mark := MarkNone
		// the newest group of this series that took the chapter says
		for j := len(c.groups) - 1; j >= 0; j-- {
			g := c.groups[j]
			if g.restored || g.url != c.url || !g.keys[k] {
				continue
			}
			mark = g.markOf(k)
			break
		}
		if (mark == MarkNone || mark == MarkCancelled) && disk[i] {
			mark = MarkDone
		}
		marks[i] = mark
	}

	return marks
}

// markOf is what became of chapter k of group g's run. A chapter that failed
// is failed even when its volume was packed without it. The caller holds c.mu.
func (g *group) markOf(k string) string {
	switch {
	case g.failed[k]:
		return MarkFailed
	case g.done[k]:
		return MarkDone
	case g.running && g.started[k]:
		return MarkDownloading
	case g.running:
		return MarkQueued
	case g.cancelling:
		return MarkCancelled
	default:
		// the run ended without saving it: its volume or bundle failed
		return MarkFailed
	}
}

// diskMarksLocked returns which chapters of the loaded list have their file
// in the folder a run of the whole list saves into. It looks again only when
// the series, the folder, the format or the file names change, or a download
// ends, so the pushes while a run goes on cost no disk access. The caller
// holds c.mu.
func (c *Controller) diskMarksLocked() []bool {
	s := c.settings
	dir := runDir(s, c.series, c.series.Chapters)
	key := fmt.Sprintf("%p\x00%s\x00%s\x00%s\x00%d", c.series, dir, s.Format, s.FilenameTemplate, c.lastEnd)
	if key == c.onDiskKey && len(c.onDisk) == len(c.series.Chapters) {
		return c.onDisk
	}
	disk := make([]bool, len(c.series.Chapters))
	if dir != "" {
		for i, chap := range c.series.Chapters {
			name, err := packer.ChapterName(c.series.Title, s.FilenameTemplate, s.Format, grabber.ChapterOf(chap))
			disk[i] = err == nil && packer.Exists(dir, name)
		}
	}
	c.onDisk, c.onDiskKey = disk, key

	return disk
}

// DownloadMissing downloads every chapter of the loaded series that isn't
// saved: no file in the folder and no download (no mark), or failed or
// cancelled. Queued and downloading chapters are left to their download,
// and locked ones can't be picked. The selection becomes those chapters,
// and they start as Download would start them, so a running download of
// the series takes them in its queue.
func (c *Controller) DownloadMissing() error {
	marks := c.ChapterMarks()
	c.mu.Lock()
	if c.loading {
		c.mu.Unlock()
		return errBusy
	}
	if c.series == nil || len(marks) != len(c.series.Chapters) {
		c.mu.Unlock()
		return uiErr("errors.loadFirst", nil, nil)
	}
	var missing []int
	for i, m := range marks {
		if (m == MarkNone || m == MarkFailed || m == MarkCancelled) && c.selectable(i) {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		// the selection stays as it was
		c.mu.Unlock()
		return uiErr("errors.nothingMissing", nil, nil)
	}
	clear(c.selected)
	for _, i := range missing {
		c.selected[i] = true
	}
	c.selGen++
	c.mu.Unlock()
	c.emit()
	return c.Start()
}
