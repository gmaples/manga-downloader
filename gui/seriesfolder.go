// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// runDir is the folder a run of chapters saves into: settings.OutputDir, or
// with SeriesFolders the series' folder inside it, tagged with the run's
// language when GuessLanguage is on too (see core.SeriesDir)
func runDir(settings *grabber.Settings, series *core.Series, chapters grabber.Filterables) string {
	if !settings.SeriesFolders || series == nil {
		return settings.OutputDir
	}
	lang := ""
	if settings.GuessLanguage {
		declared := ""
		if p, ok := series.Site.(grabber.PageLanguager); ok {
			declared = p.PageLanguage()
		}
		lang = core.FolderLanguage(chapters, settings.Language, series.Languages, series.Title, declared)
	}

	return core.SeriesDir(settings.OutputDir, series.Title, lang)
}

// Destination is the folder a run of the selected chapters would save into,
// for the window to show before it starts; "" when no series is loaded
func (c *Controller) Destination() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.series == nil {
		return ""
	}
	var chapters grabber.Filterables
	for i, chap := range c.series.Chapters {
		if i < len(c.selected) && c.selected[i] {
			chapters = append(chapters, chap)
		}
	}

	return runDir(c.settings, c.series, chapters)
}
