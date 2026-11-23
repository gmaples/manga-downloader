// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// jobFor returns the row of a chapter, nil when it has none (bundle mode has
// a single row, and a chapter whose fetch failed never got one).
// Callers must hold mu.
func (g *group) jobFor(ch *grabber.Chapter) *Job {
	if g.bundle != nil {
		return g.bundle
	}
	if j := g.chapVol[ch]; j != nil {
		return j
	}
	return g.byChap[ch]
}

// releaseLabel names a release the way the chapter list does: the language and
// group only when the loaded list has copies that need them. Callers hold mu.
func (g *group) releaseLabel(f grabber.Filterable) string {
	return core.ReleaseLabel(f, g.hasDuplicates, g.hasGroupDuplicates)
}

// releaseTags is the bracketed part of releaseLabel, which a job row shows
// apart from its title. Callers hold mu.
func (g *group) releaseTags(f grabber.Filterable) string {
	return core.ReleaseTags(f, g.hasDuplicates, g.hasGroupDuplicates)
}

// live reports whether a chapter still accepts page progress: once it failed,
// pages still in flight finish but must not move its bar. Callers hold mu.
func (g *group) live(ch *grabber.Chapter) bool {
	return !g.failedC[ch]
}

// endRow marks the row of a chapter that ended before it started, or adds one
// when it never had any. Callers hold mu.
func (g *group) endRow(ch *grabber.Chapter, state JobState, reason Message) {
	if j := g.byChap[ch]; j != nil {
		j.State = state
		j.setReason(reason)
		j.clearStatus()
		return
	}
	j := &Job{Title: grabber.ChapterLabel(ch), Tags: g.releaseTags(ch), Number: ch.GetNumber(), State: state, Key: g.keyOf(ch)}
	j.setReason(reason)
	g.jobs = append(g.jobs, j)
}

// volumeTitle is the row title of a volume: "Vol. 01 · chapters 1–7", or
// "Chapter 19 · 2 parts" in a parts run. Callers hold mu.
func (g *group) volumeTitle(v *core.Volume) Message {
	if g.runinfo.Parts {
		return msg("job.title.volumeParts", map[string]any{"volume": v.Label(), "count": len(v.Chapters)})
	}
	first, last := formatNumber(v.FirstNumber()), formatNumber(v.LastNumber())
	if first == last {
		return msg("job.title.volumeChapter", map[string]any{"volume": v.Label(), "first": first})
	}
	return msg("job.title.volume", map[string]any{"volume": v.Label(), "first": first, "last": last})
}
