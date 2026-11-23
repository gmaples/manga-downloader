// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package trash

import (
	"os"
	"path/filepath"
)

// files moves the files into ~/.Trash. A file on another volume than the
// home folder can't be moved there and is left where it is.
func files(paths []string, failed map[string]error) {
	home, err := os.UserHomeDir()
	if err != nil {
		for _, p := range paths {
			failed[p] = err
		}
		return
	}
	dir := filepath.Join(home, ".Trash")
	for _, p := range paths {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			continue
		}
		if err := os.Rename(p, FreeName(dir, filepath.Base(p))); err != nil {
			failed[p] = err
		}
	}
}
