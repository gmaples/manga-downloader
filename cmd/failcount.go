// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"sync/atomic"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// failCounter passes every event on to the renderer it wraps and counts the
// failures, so the run can end with exit code 1 when anything failed: a
// chapter whose fetch, file name, download or packing failed, and a volume
// that couldn't be packed. A file that already exists and a volume whose
// pages --skip-pages excluded aren't failures.
type failCounter struct {
	core.Events
	n atomic.Int64
}

func (f *failCounter) FetchFailed(listed grabber.Filterable, err error) {
	f.n.Add(1)
	f.Events.FetchFailed(listed, err)
}

func (f *failCounter) FilenameFailed(c *grabber.Chapter, err error) {
	f.n.Add(1)
	f.Events.FilenameFailed(c, err)
}

func (f *failCounter) DownloadFailed(c *grabber.Chapter, err error) {
	f.n.Add(1)
	f.Events.DownloadFailed(c, err)
}

func (f *failCounter) Packed(c *grabber.Chapter, name string, err error) {
	if err != nil {
		f.n.Add(1)
	}
	f.Events.Packed(c, name, err)
}

func (f *failCounter) VolumePacked(v *core.Volume, name string, err error) {
	if err != nil && !errors.Is(err, core.ErrExists) && !errors.Is(err, core.ErrSkippedVolume) {
		f.n.Add(1)
	}
	f.Events.VolumePacked(v, name, err)
}

// code is the run's exit code: 1 when anything failed, else 0
func (f *failCounter) code() int {
	if f.n.Load() > 0 {
		return 1
	}
	return 0
}
