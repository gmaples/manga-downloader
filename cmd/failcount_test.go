// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/grabber"
)

// recorder notes the events it gets, to check failCounter passes them all on
type recorder struct {
	core.NoEvents
	got []string
}

func (r *recorder) FetchFailed(grabber.Filterable, error)    { r.got = append(r.got, "fetch") }
func (r *recorder) FilenameFailed(*grabber.Chapter, error)   { r.got = append(r.got, "filename") }
func (r *recorder) DownloadFailed(*grabber.Chapter, error)   { r.got = append(r.got, "download") }
func (r *recorder) Packed(*grabber.Chapter, string, error)   { r.got = append(r.got, "packed") }
func (r *recorder) VolumePacked(*core.Volume, string, error) { r.got = append(r.got, "volume") }
func (r *recorder) Started(*grabber.Chapter)                 { r.got = append(r.got, "started") }

func TestFailCounterCode(t *testing.T) {
	ch := &grabber.Chapter{Number: 1}
	boom := errors.New("boom")
	cases := []struct {
		name string
		send func(e core.Events)
		want int
	}{
		{"nothing failed", func(e core.Events) {
			e.Started(ch)
			e.Packed(ch, "a.cbz", nil)
			e.VolumePacked(&core.Volume{}, "v.cbz", nil)
		}, 0},
		{"an existing file and a volume --skip-pages emptied", func(e core.Events) {
			e.VolumePacked(&core.Volume{}, "v.cbz", core.ErrExists)
			e.VolumePacked(&core.Volume{}, "", fmt.Errorf("x: %w", core.ErrSkippedVolume))
		}, 0},
		{"a page that failed the chapter (mangalib's 400)", func(e core.Events) {
			e.DownloadFailed(ch, errors.New("page 7: received 400 response code"))
		}, 1},
		{"a fetch", func(e core.Events) { e.FetchFailed(ch, boom) }, 1},
		{"a file name", func(e core.Events) { e.FilenameFailed(ch, boom) }, 1},
		{"a packing", func(e core.Events) { e.Packed(ch, "a.cbz", boom) }, 1},
		{"an empty volume", func(e core.Events) { e.VolumePacked(&core.Volume{}, "", core.ErrEmptyVolume) }, 1},
	}
	for _, c := range cases {
		r := &recorder{}
		f := &failCounter{Events: r}
		c.send(f)
		if got := f.code(); got != c.want {
			t.Errorf("%s: code = %d, want %d", c.name, got, c.want)
		}
	}

	// every event reaches the renderer
	r := &recorder{}
	f := &failCounter{Events: r}
	f.Started(ch)
	f.FetchFailed(ch, boom)
	f.FilenameFailed(ch, boom)
	f.DownloadFailed(ch, boom)
	f.Packed(ch, "a", nil)
	f.VolumePacked(&core.Volume{}, "v", nil)
	if want := "[started fetch filename download packed volume]"; fmt.Sprint(r.got) != want {
		t.Errorf("renderer got %v, want %s", r.got, want)
	}
}
