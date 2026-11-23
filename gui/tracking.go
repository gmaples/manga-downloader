// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"context"
	"errors"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// The fork's part of each event (see events in controller.go): the chapters
// in flight for the site bars, the started, failed and done marks of the
// group's chapters, the history saves, the VPN hints and the log's start
// lines. Each is called where its lines were, under c.mu.

func (g *group) trackFetching(c *Controller, listed grabber.Filterable) {
	c.flightSeq++
	g.flightOf[listed] = &flight{seq: c.flightSeq, title: grabber.ChapterLabel(listed), tags: g.releaseTags(listed)}
	g.started[chapKey(listed)] = true
}

func (g *group) trackFetched(listed grabber.Filterable, ch *grabber.Chapter) {
	if f := g.flightOf[listed]; f != nil {
		f.title, f.tags = grabber.ChapterLabel(ch), g.releaseTags(ch)
		g.flights[ch] = f
		delete(g.flightOf, listed)
	}
	g.listedOf[ch] = listed
}

func (g *group) trackFetchFailed(c *Controller, listed grabber.Filterable, err error) {
	delete(g.flightOf, listed)
	g.failed[chapKey(listed)] = true
	c.noteVPNLocked(g.siteKey, err)
}

func (g *group) trackSkipped(c *Controller, ch *grabber.Chapter) {
	delete(g.flights, ch)
	g.markDone(ch)
	c.kickHistory()
}

func (g *group) trackExists(c *Controller, listed grabber.Filterable) {
	g.done[chapKey(listed)] = true
	c.kickHistory()
	delete(g.flightOf, listed)
	if ch, ok := listed.(*grabber.Chapter); ok {
		delete(g.flights, ch)
	}
}

func (g *group) trackFilenameFailed(ch *grabber.Chapter) {
	delete(g.flights, ch)
	g.failed[g.keyOf(ch)] = true
}

func (g *group) trackStarted(c *Controller, ch *grabber.Chapter) {
	g.started[g.keyOf(ch)] = true
	f := g.flights[ch]
	if f == nil {
		c.flightSeq++
		f = &flight{seq: c.flightSeq, title: grabber.ChapterLabel(ch), tags: g.releaseTags(ch)}
		g.flights[ch] = f
	}
	// a chapter packed on its own also counts its archive steps, as its row does
	f.done, f.total = 0, ch.PagesCount*core.DownloadWeight
	if g.bundle == nil && g.chapVol[ch] == nil {
		f.total += ch.PagesCount * core.ArchiveWeight
	}
	c.appendMsgLocked(LogInfo, msg("log.started", map[string]any{"label": g.releaseLabel(ch), "pages": ch.PagesCount}))
}

func (g *group) trackPage(ch *grabber.Chapter, err error) {
	if f := g.flights[ch]; f != nil && err == nil && g.live(ch) {
		f.done += core.DownloadWeight
	}
}

func (g *group) trackDownloadFailed(c *Controller, ch *grabber.Chapter, err error) {
	g.failed[g.keyOf(ch)] = true
	delete(g.flights, ch)
	c.noteVPNLocked(g.siteKey, err)
}

func (g *group) trackArchived(ch *grabber.Chapter) {
	if f := g.flights[ch]; f != nil {
		f.done += core.ArchiveWeight
	}
}

func (g *group) trackPacked(c *Controller, ch *grabber.Chapter, err error) {
	delete(g.flights, ch)
	if err != nil {
		g.failed[g.keyOf(ch)] = true
		return
	}
	g.markDone(ch)
	c.kickHistory()
}

func (g *group) trackDownloaded(c *Controller, ch *grabber.Chapter) {
	delete(g.flights, ch)
	c.noteVPNLocked(g.siteKey, nil)
}

// trackVolumePacked marks volume v's chapters done when it was saved, left
// out (its file is there) or skipped whole, as VolumePacked's cases tell
func (g *group) trackVolumePacked(c *Controller, v *core.Volume, err error) {
	switch {
	case errors.Is(err, context.Canceled):
	case errors.Is(err, core.ErrSkippedVolume), errors.Is(err, core.ErrExists), err == nil:
		g.markVolumeDone(v)
	}
	c.kickHistory()
}
