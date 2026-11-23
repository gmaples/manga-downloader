// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/trash"
)

// Waiting is a chapter waiting for another copy of the program, which is
// downloading the same file: only the log says so, the row stays as it is
func (e events) Waiting(ch *grabber.Chapter, name string) {
	c, g := e.c, e.g
	c.mu.Lock()
	c.appendMsgLocked(LogInfo, msg("log.waiting", map[string]any{"label": g.releaseLabel(ch), "name": name}))
	c.mu.Unlock()
	c.emit()
}

// Correlating is a step of the folder's correlation, as it goes: the
// group's header says what it's doing, with its count, and the log gets the
// folder's file count, the files read every core.CorrelateLogEvery, and each file
// moved (or not) as its batch is done.
func (e events) Correlating(p core.CorrelateProgress) {
	c, g := e.c, e.g
	c.mu.Lock()
	var m Message
	switch p.Step {
	case core.CorrelateListed:
		m = msg("activity.status.listed", map[string]any{"count": p.Total})
		c.appendMsgLocked(LogInfo, msg("log.correlating", map[string]any{"count": p.Total}))
	case core.CorrelateComparing:
		m = msg("activity.status.comparing", map[string]any{"done": p.Done, "total": p.Total})
		if p.Done%core.CorrelateLogEvery == 0 || p.Done == p.Total {
			c.appendMsgLocked(LogInfo, msg("log.compared", map[string]any{"done": p.Done, "total": p.Total}))
		}
	case core.CorrelateMoving:
		m = msg("activity.status.moving", map[string]any{"done": p.Done, "total": p.Total})
		// deleted for good, the lines say so; kept ones are summed up once
		for _, d := range p.Moved {
			args := map[string]any{"name": d.Name, "kept": d.Kept}
			switch {
			case errors.Is(d.Err, trash.ErrNoRecycleBin):
			case d.Err != nil:
				c.appendMsgLocked(LogWarn, msg("log.removeFailed", map[string]any{"name": d.Name, "reason": d.Err.Error()}))
			case d.SameBytes && g.forGood:
				c.appendMsgLocked(LogInfo, msg("log.removedSameForGood", args))
			case d.SameBytes:
				c.appendMsgLocked(LogInfo, msg("log.removedSame", args))
			case g.forGood:
				args["number"] = d.Number
				c.appendMsgLocked(LogInfo, msg("log.removedNumberForGood", args))
			default:
				args["number"] = d.Number
				c.appendMsgLocked(LogInfo, msg("log.removedNumber", args))
			}
		}
	}
	g.status = &m
	c.mu.Unlock()
	c.emit()
}

// Correlated is what the correlation of the folder did before the download
// started: the group's header says how many files went (a later pass of the
// group adds to it), its status ends, and the log gets the total.
func (e events) Correlated(r *core.Correlation) {
	c, g := e.c, e.g
	c.mu.Lock()
	if g.cleanup == nil {
		g.cleanup = &Cleanup{}
	}
	g.cleanup.Removed += len(r.Removed)
	g.cleanup.Bytes += r.RemovedBytes
	g.cleanup.Permanent = g.cleanup.Permanent || r.Permanent
	kept := 0
	for _, d := range r.Failed {
		if errors.Is(d.Err, trash.ErrNoRecycleBin) {
			kept++
		}
	}
	g.cleanup.Kept += kept
	g.cleanup.Failed += len(r.Failed) - kept
	g.status = nil
	removed := map[string]any{"count": len(r.Removed), "size": core.Bytes(r.RemovedBytes), "folder": r.Dir}
	switch {
	case len(r.Removed) == 0:
	case r.Permanent:
		c.appendMsgLocked(LogInfo, msg("log.removedForGood", removed))
	default:
		c.appendMsgLocked(LogInfo, msg("log.removed", removed))
	}
	if kept > 0 {
		c.appendMsgLocked(LogWarn, msg("log.keptNoBin", map[string]any{"count": kept, "folder": r.Dir}))
	}
	// the marks look at the folder again
	c.onDiskKey = ""
	c.mu.Unlock()
	c.emit()
}
