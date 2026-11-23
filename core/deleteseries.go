// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/trash"
)

// Deleting a series takes its files out of the folder it downloads into, a
// batch at a time so the count moves: the app moves them to the system's
// trash (the Recycle Bin on Windows), and the command line moves them into an
// EmptyMeDir folder next to them, unless --force (elevated only) sends them to
// the trash too.

// EmptyMeDir is the folder of the download root the command line moves a
// deleted series into, one folder per series, for the user to empty
const EmptyMeDir = "[Empty Me]"

// deleteBatch is how many files go in one trash call or one progress step
const deleteBatch = 100

// ErrNothingToDelete is a delete of a series with no file in its folder
var ErrNothingToDelete = errors.New("no file of the series in the folder")

// DeletePlan is what a delete of a series would take from a folder
type DeletePlan struct {
	// Dir is the folder
	Dir string
	// Whole is a series folder taken whole: every file and folder in it, then
	// the folder itself once empty. Otherwise only the series' chapter files
	// go, as the folder holds other series' files too.
	Whole bool
	// Names are the files (and, whole, the folders) in Dir that go, in name order
	Names []string
	// Bytes is their size
	Bytes int64
	// Permanent is a folder on a drive with no Recycle Bin: a delete to the
	// trash would delete the files for good, so Run asks for leave
	Permanent bool
	// sizes are each name's size
	sizes map[string]int64
}

// DeleteProgress is a delete's count: Done of Total files handled so far,
// Failed of those not moved
type DeleteProgress struct {
	Done, Total, Failed int
}

// DeleteResult is what a delete did
type DeleteResult struct {
	// Moved are the names moved, Failed the names left with their error
	Moved  []string
	Failed map[string]error
	// Bytes is the size moved
	Bytes int64
	// To is where they went: "" for the trash, else the folder
	To string
	// Permanent is set when they were deleted for good (DeletePlan.Permanent)
	Permanent bool
	// DirRemoved is a whole series folder removed once empty
	DirRemoved bool
}

// DeletePlan lists what deleting the series takes from dir: with whole (dir
// is the series' own folder) everything in it, else the series' chapter
// files, the ones its file names give a chapter number of this series (see
// packer.NewNumberMatcher), whatever their title or group. A file another
// copy of the program is writing (it has a claim, see ClaimSuffix) makes the
// plan fail, as that copy is downloading the series. ErrNothingToDelete when
// nothing would go.
func (s *Series) DeletePlan(dir string, whole bool) (*DeletePlan, error) {
	p := &DeletePlan{Dir: dir, Whole: whole, sizes: map[string]int64{}}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return p, ErrNothingToDelete
		}
		return p, err
	}
	var matcher *packer.NumberMatcher
	if !whole {
		var ok bool
		matcher, ok = packer.NewNumberMatcher(s.Title, s.Site.GetFilenameTemplate(), s.Site.GetFormat())
		if !ok {
			return p, ErrNothingToDelete
		}
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ClaimSuffix) {
			if whole || isSeriesFile(matcher, strings.TrimSuffix(name, ClaimSuffix)) {
				return p, ErrSeriesBusy
			}
			continue
		}
		if !whole && (!e.Type().IsRegular() || !isSeriesFile(matcher, name)) {
			continue
		}
		size := sizeOf(filepath.Join(dir, name), e)
		p.Names = append(p.Names, name)
		p.sizes[name] = size
		p.Bytes += size
	}
	sort.Strings(p.Names)
	if len(p.Names) == 0 {
		return p, ErrNothingToDelete
	}
	p.Permanent = !trash.HasRecycleBin(filepath.Join(dir, p.Names[0]))
	return p, nil
}

// ErrSeriesBusy is a delete of a series another copy of the program is
// downloading into the folder
var ErrSeriesBusy = errors.New("another copy of the program is downloading into the folder")

// isSeriesFile tells whether name is one of the series' chapter files
func isSeriesFile(m *packer.NumberMatcher, name string) bool {
	_, ok := m.Number(name)
	return ok
}

// sizeOf is a file's size, or a folder's files' sizes added up
func sizeOf(path string, e os.DirEntry) int64 {
	if !e.IsDir() {
		info, err := e.Info()
		if err != nil {
			return 0
		}
		return info.Size()
	}
	var n int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return n
}

// EmptyMeFolder is the folder the command line moves the series' files from
// root into: root/[Empty Me]/<series>
func EmptyMeFolder(root, title string) string {
	return filepath.Join(root, EmptyMeDir, packer.SanitizeDirname(title, packer.MaxNameBytes))
}

// Run takes the plan's files out of its folder, a batch at a time: to the
// system's trash when to is "", else into the folder to (made if needed), a
// name already there getting a " (2)" and so on. progress, when not nil, is
// told the count after each batch. A whole series folder left empty is then
// removed. When the plan is Permanent, a delete to the trash deletes the
// files for good if allowPermanent, and otherwise touches nothing and fails
// every name with trash.ErrNoRecycleBin.
func (p *DeletePlan) Run(to string, allowPermanent bool, progress func(DeleteProgress)) *DeleteResult {
	if progress == nil {
		progress = func(DeleteProgress) {}
	}
	r := &DeleteResult{To: to, Failed: map[string]error{}}
	remove := trash.Files
	if to == "" && p.Permanent {
		if !allowPermanent {
			for _, n := range p.Names {
				r.Failed[n] = trash.ErrNoRecycleBin
			}
			progress(DeleteProgress{Done: len(p.Names), Total: len(p.Names), Failed: len(p.Names)})
			return r
		}
		remove, r.Permanent = trash.Remove, true
	}
	if to != "" {
		if err := os.MkdirAll(to, 0o755); err != nil {
			for _, n := range p.Names {
				r.Failed[n] = err
			}
			progress(DeleteProgress{Done: len(p.Names), Total: len(p.Names), Failed: len(p.Names)})
			return r
		}
	}
	sizes := p.sizes
	for start := 0; start < len(p.Names); start += deleteBatch {
		batch := p.Names[start:min(start+deleteBatch, len(p.Names))]
		if to == "" {
			paths := make([]string, len(batch))
			for i, n := range batch {
				paths[i] = filepath.Join(p.Dir, n)
			}
			failed := remove(paths)
			for i, n := range batch {
				if err := failed[paths[i]]; err != nil {
					r.Failed[n] = err
				} else {
					r.Moved = append(r.Moved, n)
					r.Bytes += sizes[n]
				}
			}
		} else {
			for _, n := range batch {
				dest := freeDest(to, n)
				if err := os.Rename(filepath.Join(p.Dir, n), dest); err != nil {
					r.Failed[n] = err
				} else {
					r.Moved = append(r.Moved, n)
					r.Bytes += sizes[n]
				}
			}
		}
		progress(DeleteProgress{Done: start + len(batch), Total: len(p.Names), Failed: len(r.Failed)})
	}
	if p.Whole && len(r.Failed) == 0 {
		// only an empty folder goes: os.Remove refuses one with anything left
		if err := os.Remove(p.Dir); err == nil {
			r.DirRemoved = true
		}
	}
	return r
}

// freeDest is name in dir, or "name (2).ext" and so on when that's taken
func freeDest(dir, name string) string {
	dest := filepath.Join(dir, name)
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		return dest
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		dest = filepath.Join(dir, stem+" ("+strconv.Itoa(i)+")"+ext)
		if _, err := os.Lstat(dest); os.IsNotExist(err) {
			return dest
		}
	}
}
