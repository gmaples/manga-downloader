// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package trash moves files to the system's Recycle Bin (Windows) or Trash
// (macOS, and the freedesktop.org trash of Linux and other systems), so a file
// removed by the program can be brought back.
package trash

// Files moves each file at paths (absolute paths) to the system's trash and
// returns the paths it couldn't move, with the error. A file that's already
// gone counts as moved.
func Files(paths []string) (failed map[string]error) {
	failed = map[string]error{}
	if len(paths) == 0 {
		return failed
	}
	files(paths, failed)
	return failed
}
