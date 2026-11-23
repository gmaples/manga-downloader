// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package trash moves files to the system's Recycle Bin (Windows) or Trash
// (macOS, and the freedesktop.org trash of Linux and other systems), so a file
// removed by the program can be brought back.
package trash

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoRecycleBin is what Files returns for a file on a drive that has no
// Recycle Bin or trash: the file is left where it is
var ErrNoRecycleBin = errors.New("this drive has no Recycle Bin")

// hasRecycleBin is the system's check, a variable so tests can stub it
var hasRecycleBin = osHasRecycleBin

// HasRecycleBin reports whether a file at path would go to a Recycle Bin or
// trash it can be restored from, rather than being deleted for good
func HasRecycleBin(path string) bool { return hasRecycleBin(path) }

// Files moves each file at paths (absolute paths) to the system's trash and
// returns the paths it couldn't move, with the error. A file that's already
// gone counts as moved. A file on a drive with no Recycle Bin isn't touched
// and fails with ErrNoRecycleBin: see Remove to delete those.
func Files(paths []string) (failed map[string]error) {
	failed = map[string]error{}
	if len(paths) == 0 {
		return failed
	}
	// the check is asked once per drive
	bin := map[string]bool{}
	var keep []string
	for _, p := range paths {
		vol := filepath.VolumeName(p)
		ok, seen := bin[vol]
		if !seen {
			ok = hasRecycleBin(p)
			bin[vol] = ok
		}
		if ok {
			keep = append(keep, p)
		} else if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			failed[p] = ErrNoRecycleBin
		}
	}
	if len(keep) > 0 {
		files(keep, failed)
	}
	return failed
}

// Remove deletes the files at paths for good, and returns the ones it
// couldn't delete, with the error. A file that's already gone counts as removed.
func Remove(paths []string) (failed map[string]error) {
	failed = map[string]error{}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			failed[p] = err
		}
	}
	return failed
}
