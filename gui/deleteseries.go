// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"sort"

	"github.com/elboletaire/manga-downloader/core"
)

// Delete series (the chapter list's second count line) takes the loaded
// series' files out of the folder its downloads go to, to the Recycle Bin or
// trash, a batch at a time with the count in the log, after the window asks
// to confirm with what DeleteInfo says. The series leaves the history; its
// rows in the Activity pane stay.

// DeleteInfo is what a delete of the loaded series would take, for the
// window to confirm
type DeleteInfo struct {
	Title string `json:"title"`
	// Dir is the folder the files are in
	Dir string `json:"dir"`
	// Whole is a series folder taken whole, the folder too
	Whole bool `json:"whole"`
	Files int  `json:"files"`
	// Size is the files' size, written to read (1.4 GB)
	Size string `json:"size"`
	// Permanent is a folder on a drive with no Recycle Bin: the files would
	// be deleted for good, so the window warns and DeleteSeries needs leave
	Permanent bool `json:"permanent"`
}

// deletePlanLocked plans the delete of the loaded series: with series
// folders, its folder whole, else its chapter files in the output folder.
// The caller holds c.mu.
func (c *Controller) deletePlanLocked() (*core.DeletePlan, error) {
	if c.loading {
		return nil, errBusy
	}
	if c.series == nil {
		return nil, uiErr("errors.loadFirst", nil, nil)
	}
	if c.seriesGroupLocked() != nil {
		return nil, uiErr("errors.deleteRunning", nil, nil)
	}
	dir := runDir(c.settings, c.series, c.series.Chapters)
	plan, err := c.series.DeletePlan(dir, c.settings.SeriesFolders)
	switch {
	case errors.Is(err, core.ErrNothingToDelete):
		return nil, uiErr("errors.deleteNothing", map[string]any{"folder": dir}, err)
	case errors.Is(err, core.ErrSeriesBusy):
		return nil, uiErr("errors.deleteBusy", map[string]any{"folder": dir}, err)
	case err != nil:
		return nil, uiErr("errors.deleteFailed", map[string]any{"reason": err.Error()}, err)
	}
	return plan, nil
}

// DeleteInfo says what deleting the loaded series would take
func (c *Controller) DeleteInfo() (*DeleteInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	plan, err := c.deletePlanLocked()
	if err != nil {
		return nil, err
	}
	return &DeleteInfo{Title: c.series.Title, Dir: plan.Dir, Whole: plan.Whole, Files: len(plan.Names), Size: sizeText(plan.Bytes), Permanent: plan.Permanent}, nil
}

// DeleteSeries moves the loaded series' files to the trash, planned again
// now so it takes what's there, and logs each batch's count, each file it
// couldn't move, and "Deleted series <title> from <folder>". progress, when
// not nil, is told the count after each batch. The series leaves the
// history. On a drive with no Recycle Bin the files are deleted for good,
// only with permanentOK (the window warned), and otherwise nothing is.
func (c *Controller) DeleteSeries(permanentOK bool, progress func(core.DeleteProgress)) error {
	c.mu.Lock()
	plan, err := c.deletePlanLocked()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	// planned again: the window's warning may not have been about this drive
	if plan.Permanent && !permanentOK {
		c.mu.Unlock()
		return uiErr("errors.deletePermanentNotConfirmed", nil, nil)
	}
	title, url := c.series.Title, c.url
	deleting := map[string]any{"title": title, "count": len(plan.Names), "size": sizeText(plan.Bytes), "folder": plan.Dir}
	if plan.Permanent {
		c.appendMsgLocked(LogInfo, msg("log.deletingForGood", deleting))
	} else {
		c.appendMsgLocked(LogInfo, msg("log.deleting", deleting))
	}
	c.mu.Unlock()
	c.emit()

	r := plan.Run("", permanentOK, func(p core.DeleteProgress) {
		count := map[string]any{"done": p.Done, "total": p.Total}
		if plan.Permanent {
			c.addMsg(LogInfo, msg("log.deleteMovedForGood", count))
		} else {
			c.addMsg(LogInfo, msg("log.deleteMoved", count))
		}
		if progress != nil {
			progress(p)
		}
	})

	c.mu.Lock()
	failed := make([]string, 0, len(r.Failed))
	for n := range r.Failed {
		failed = append(failed, n)
	}
	sort.Strings(failed)
	for _, n := range failed {
		c.appendMsgLocked(LogWarn, msg("log.deleteFileFailed", map[string]any{"name": n, "reason": r.Failed[n].Error()}))
	}
	if len(r.Moved) > 0 {
		c.appendMsgLocked(LogInfo, msg("log.deletedSeries", map[string]any{"title": title, "folder": plan.Dir, "count": len(r.Moved), "size": sizeText(r.Bytes)}))
	}
	if len(failed) > 0 {
		c.appendMsgLocked(LogWarn, msg("log.deleteIncomplete", map[string]any{"count": len(failed)}))
	}
	// the series leaves the history, so it isn't offered again
	kept := c.history[:0]
	for _, e := range c.history {
		if e.URL != url {
			kept = append(kept, e)
		}
	}
	c.history = kept
	// what this session's downloads saved is gone: they no longer mark the list
	for _, g := range c.groups {
		if g.url == url {
			g.deleted = true
		}
	}
	// the marks look at the folder again
	c.onDiskKey = ""
	c.mu.Unlock()
	c.kickHistory()
	c.emit()
	if len(failed) > 0 {
		return uiErr("errors.deleteIncomplete", map[string]any{"count": len(failed)}, nil)
	}
	return nil
}
