// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"

	"github.com/elboletaire/manga-downloader/core"
)

// GroupDTO is one download group: one press of Download, with its own rows,
// cancel and summary (null while it runs)
type GroupDTO struct {
	ID         int         `json:"id"`
	Title      string      `json:"title"`
	Running    bool        `json:"running"`
	Cancelling bool        `json:"cancelling"`
	Run        RunDTO      `json:"run"`
	Jobs       []JobDTO    `json:"jobs"`
	Summary    *SummaryDTO `json:"summary"`
	// Restored is set on a group of the last session, read back from
	// history.txt: Resume loads its series with its to-do chapters selected
	Restored bool   `json:"restored"`
	URL      string `json:"url"`
	// Adult is set when the site rates the series 18+
	Adult bool `json:"adult"`
	// Unfinished counts the chapters Restart(ID, "") downloads again
	Unfinished int `json:"unfinished"`
	// Cleanup is what the correlation of the group's folder did when its
	// download started, nil when it didn't run
	Cleanup *CleanupDTO `json:"cleanup"`
	// Status is what the group is doing that no row shows (looking for
	// duplicates, moving them), nil when nothing
	Status *Message `json:"status"`
}

// CleanupDTO is the duplicate files a group's download moved to the Recycle
// Bin when it started: how many, their size in bytes, and how many couldn't be
type CleanupDTO struct {
	Removed   int   `json:"removed"`
	Bytes     int64 `json:"bytes"`
	Failed    int   `json:"failed"`
	Permanent bool  `json:"permanent"`
	Kept      int   `json:"kept"`
}

// SiteBarDTO is a SiteBar for the window. Until is the end of the site's
// pause after a 429 in Unix milliseconds, 0 when it isn't paused, so the
// window can count it down on its own. Parallel is how many of the site's
// requests may be under way at once, 0 before its first request.
type SiteBarDTO struct {
	Site  string `json:"site"`
	Title string `json:"title"`
	Tags  string `json:"tags"`
	Done  int64  `json:"done"`
	Total int64  `json:"total"`
	Until int64  `json:"until"`
	// Parallel is SiteBar.Parallel
	Parallel int `json:"parallel"`
	// VPN is SiteBar.VPN
	VPN string `json:"vpn"`
}

func siteBarDTO(b SiteBar) SiteBarDTO {
	d := SiteBarDTO{Site: b.Site, Title: b.Title, Tags: b.Tags, Done: b.Done, Total: b.Total, Parallel: b.Parallel, VPN: b.VPN}
	if !b.Until.IsZero() {
		d.Until = b.Until.UnixMilli()
	}
	return d
}

// Marks returns the mark of each chapter of the list, by index: "" (no file
// and no download), "done", "downloading", "queued", "failed" or "cancelled"
// (see Controller.ChapterMarks). It's empty when nothing is loaded.
func (a *App) Marks() []string {
	m := a.ctrl.ChapterMarks()
	if m == nil {
		return []string{}
	}
	return m
}

// MarkNotes returns, by index, what each chapter's mark can say more: the
// file of a chapter that's in the folder, the reason of a failed one, else ""
// (see Controller.ChapterNotes). It's empty when nothing is loaded.
func (a *App) MarkNotes() []string {
	n := a.ctrl.ChapterNotes()
	if n == nil {
		return []string{}
	}
	return n
}

// SetPaneSizes saves the sizes, in CSS pixels, the user dragged the side panel
// and the log drawer to; 0, or a size out of range, keeps the default. It's
// never refused while busy.
func (a *App) SetPaneSizes(sideWidth, logHeight int) error {
	a.ctrl.SetPrefs(func(p *PrefsDTO) { p.SideWidth, p.LogHeight = paneSize(sideWidth), paneSize(logHeight) })
	a.save()

	return nil
}

// openFolderDir picks the folder OpenFolder opens: the last run's, else the
// destination when it exists, else the output folder
func openFolderDir(outputDir, destination string, last *Summary) string {
	if last != nil && last.Dir != "" {
		return last.Dir
	}
	if destination != "" && destination != outputDir {
		if fi, err := os.Stat(destination); err == nil && fi.IsDir() {
			return destination
		}
	}

	return outputDir
}

// HistoryDTO is one series of the history pop-up. When is Unix
// milliseconds, 0 when unknown; Done and Todo count its chapters.
type HistoryDTO struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	// Site is the site's home page, "" when the URL has none
	Site  string `json:"site"`
	Adult bool   `json:"adult"`
	When  int64  `json:"when"`
	Done  int    `json:"done"`
	Todo  int    `json:"todo"`
}

// History returns the series of history.txt, newest first
func (a *App) History() []HistoryDTO {
	out := []HistoryDTO{}
	for _, e := range a.ctrl.History() {
		d := HistoryDTO{Title: e.Title, URL: e.URL, Site: historySite(e.URL), Adult: e.Adult, Todo: e.Todo()}
		d.Done = len(e.Chapters) - d.Todo
		if !e.When.IsZero() {
			d.When = e.When.UnixMilli()
		}
		out = append(out, d)
	}
	return out
}

// ClearHistory empties history.txt
func (a *App) ClearHistory() {
	a.ctrl.ClearHistory()
}

// Resume loads the series of restored group id with its to-do chapters
// selected, ready for Download; it downloads nothing
func (a *App) Resume(id int) error {
	url, load, err := a.ctrl.Resume(id)
	if err != nil || !load {
		return err
	}
	return a.Load(url)
}

// Restart downloads again the unfinished chapters of group id: the row's
// with its key, all of them with key "" (see Controller.Restart). When the
// group's series isn't the one loaded, it's loaded first, and starts once
// it is.
func (a *App) Restart(id int, key string) error {
	url, load, err := a.ctrl.Restart(id, key)
	if err != nil || !load {
		return err
	}
	return a.Load(url)
}

// DownloadMissing selects and downloads every chapter of the loaded series
// that isn't saved, or failed or was cancelled (see Controller.DownloadMissing)
func (a *App) DownloadMissing() error {
	return a.ctrl.DownloadMissing()
}

// DeleteInfo says what Delete series would take: the folder, its file count
// and size, for the window to confirm (see Controller.DeleteInfo)
func (a *App) DeleteInfo() (*DeleteInfo, error) {
	return a.ctrl.DeleteInfo()
}

// DeleteSeries moves the loaded series' files to the Recycle Bin or trash,
// pushing "deleteProgress" ({done, total, failed}) after each batch (see
// Controller.DeleteSeries); permanentOK is the window's leave to delete for
// good on a drive with no Recycle Bin (DeleteInfo.Permanent)
func (a *App) DeleteSeries(permanentOK bool) error {
	return a.ctrl.DeleteSeries(permanentOK, func(p core.DeleteProgress) {
		a.sink.Emit("deleteProgress", map[string]int{"done": p.Done, "total": p.Total, "failed": p.Failed})
	})
}

// AnswerPermanent answers the dedupe's question id, State.Permanent: yes
// deletes the duplicates for good (see Controller.AnswerPermanent)
func (a *App) AnswerPermanent(id int, yes bool) error {
	return a.ctrl.AnswerPermanent(id, yes)
}

// CancelGroup stops group id from starting new chapters
func (a *App) CancelGroup(id int) {
	a.ctrl.CancelGroup(id)
}

func groupDTO(g GroupState) GroupDTO {
	out := GroupDTO{
		ID:         g.ID,
		Title:      g.Title,
		Running:    g.Running,
		Cancelling: g.Cancelling,
		Run:        runDTO(g.Run),
		Jobs:       []JobDTO{},
		Summary:    summaryDTO(g.Summary),
		Restored:   g.Restored,
		URL:        g.URL,
		Adult:      g.Adult,
		Unfinished: g.Unfinished,
	}
	out.Status = g.Status
	if g.Cleanup != nil {
		out.Cleanup = &CleanupDTO{Removed: g.Cleanup.Removed, Bytes: g.Cleanup.Bytes, Failed: g.Cleanup.Failed, Permanent: g.Cleanup.Permanent, Kept: g.Cleanup.Kept}
	}
	for _, j := range g.Jobs {
		out.Jobs = append(out.Jobs, JobDTO{
			Title:     j.Title,
			TitleMsg:  j.TitleMsg,
			Tags:      j.Tags,
			Number:    j.Number,
			Total:     j.Total,
			Done:      j.Done,
			State:     jobStateName(j.State),
			Reason:    j.Reason,
			ReasonMsg: j.ReasonMsg,
			Status:    j.Status,
			StatusMsg: j.StatusMsg,
			Volume:    j.Volume,
			Key:       j.Key,
			Restart:   j.Restart,
		})
	}

	return out
}
