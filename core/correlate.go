// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
	"github.com/elboletaire/manga-downloader/trash"
)

// With --skip-existing, a download of chapters packed one file each first
// correlates the series' folder with what it's about to download (see
// Series.Correlate): two files with the same bytes are one chapter whatever
// their names, and two files of one chapter number are one chapter whatever
// their title or group (a site that renamed its chapters, or another group's
// release). The oldest file of each is kept and the others are moved to the
// system's trash (the Recycle Bin on Windows), and a chapter whose number is
// already in the folder isn't downloaded again.

// Correlation is what Series.Correlate found and did in a folder
type Correlation struct {
	// Dir is the folder
	Dir string
	// Removed are the files moved to the trash, in name order
	Removed []Duplicate
	// RemovedBytes is their size
	RemovedBytes int64
	// Failed are the duplicates that couldn't be moved, with the error; they
	// stay in the folder
	Failed []Duplicate
	// Have are the chapter files in the folder by chapter number (as
	// packer.NumberText writes it), after the duplicates went
	Have map[string]string
}

// Duplicate is a file found to be the same chapter as Kept, a file kept in
// the folder: SameBytes when their bytes are equal, else by chapter number
type Duplicate struct {
	Name      string
	Kept      string
	Number    string
	SameBytes bool
	Size      int64
	Err       error
}

// folderFile is a file of the folder that may be a chapter
type folderFile struct {
	name   string
	size   int64
	mod    time.Time
	number string
	hash   [sha256.Size]byte
	hashed bool
}

// older orders the files to keep first: the oldest, then the shortest name
// (an original before its " v2"), then by name
func older(a, b *folderFile) bool {
	if !a.mod.Equal(b.mod) {
		return a.mod.Before(b.mod)
	}
	if len(a.name) != len(b.name) {
		return len(a.name) < len(b.name)
	}
	return a.name < b.name
}

// Correlate finds the duplicate chapter files in dir, moves them to the
// trash and returns what it found. Files with the format's extension are
// compared by their bytes (only those whose size another one shares are
// read), unless their names give two different chapter numbers; the series'
// chapter files are then grouped by the number in their
// name (packer.NumberMatcher). A file another copy of the program is writing
// (it has a claim, see ClaimSuffix) is left alone. A dir that doesn't exist
// yet has nothing to correlate.
func (s *Series) Correlate(dir string) (*Correlation, error) {
	r := &Correlation{Dir: dir, Have: map[string]string{}}
	format := s.Site.GetFormat()
	matcher, byNumber := packer.NewNumberMatcher(s.Title, s.Site.GetFilenameTemplate(), format)
	if format == packer.FormatRaw {
		return r, nil
	}
	ext := extOf(format)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return r, nil
		}
		return r, err
	}
	claimed := map[string]bool{}
	for _, e := range entries {
		if n := e.Name(); strings.HasSuffix(n, ClaimSuffix) {
			claimed[strings.TrimSuffix(n, ClaimSuffix)] = true
		}
	}
	var files []*folderFile
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || claimed[name] || !strings.EqualFold(filepath.Ext(name), ext) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		f := &folderFile{name: name, size: info.Size(), mod: info.ModTime()}
		if byNumber {
			f.number, _ = matcher.Number(name)
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return older(files[i], files[j]) })

	removed := map[*folderFile]bool{}
	var dups []Duplicate

	// same bytes: only sizes two files share are worth reading, and an empty
	// file is no chapter's
	bySize := map[int64][]*folderFile{}
	for _, f := range files {
		bySize[f.size] = append(bySize[f.size], f)
	}
	for _, f := range files {
		same := bySize[f.size]
		if len(same) < 2 || removed[f] || f.size == 0 {
			continue
		}
		if !f.hashed && !hashFile(dir, f) {
			continue
		}
		for _, g := range same {
			// two chapter numbers the names say apart aren't one chapter
			// even with the same bytes: a site that serves one placeholder
			// for its locked chapters would otherwise lose all but one
			if g == f || removed[g] || !older(f, g) || f.number != "" && g.number != "" && f.number != g.number {
				continue
			}
			if !g.hashed && !hashFile(dir, g) {
				continue
			}
			if g.hash == f.hash {
				removed[g] = true
				dups = append(dups, Duplicate{Name: g.name, Kept: f.name, Number: g.number, SameBytes: true, Size: g.size})
			}
		}
	}

	// same chapter number: the oldest file of each is kept
	kept := map[string]*folderFile{}
	for _, f := range files {
		if removed[f] || f.number == "" {
			continue
		}
		if k, ok := kept[f.number]; ok {
			removed[f] = true
			dups = append(dups, Duplicate{Name: f.name, Kept: k.name, Number: f.number, Size: f.size})
			continue
		}
		kept[f.number] = f
	}

	paths := make([]string, len(dups))
	for i, d := range dups {
		paths[i] = filepath.Join(dir, d.Name)
	}
	failed := trash.Files(paths)
	for i, d := range dups {
		if err := failed[paths[i]]; err != nil {
			d.Err = err
			r.Failed = append(r.Failed, d)
			continue
		}
		r.Removed = append(r.Removed, d)
		r.RemovedBytes += d.Size
	}
	sort.Slice(r.Removed, func(i, j int) bool { return r.Removed[i].Name < r.Removed[j].Name })
	for n, f := range kept {
		r.Have[n] = f.name
	}
	// a duplicate that couldn't go still holds its number when its kept file is gone
	for _, d := range r.Failed {
		if d.Number != "" && r.Have[d.Number] == "" {
			r.Have[d.Number] = d.Name
		}
	}

	return r, nil
}

// hashFile reads f's SHA-256, false when it can't be read
func hashFile(dir string, f *folderFile) bool {
	h, err := os.Open(filepath.Join(dir, f.name))
	if err != nil {
		return false
	}
	defer h.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, h); err != nil {
		return false
	}
	copy(f.hash[:], sum.Sum(nil))
	f.hashed = true
	return true
}

// extOf is the extension of a format's files
func extOf(format string) string {
	switch format {
	case packer.FormatEPUB:
		return ".epub"
	case packer.FormatCBR:
		return ".cbr"
	default:
		return ".cbz"
	}
}

// numberOf is a listed chapter's number as file names write it, the key of
// Correlation.Have
func numberOf(f grabber.Filterable) string {
	return packer.NumberText(f.GetNumber())
}
