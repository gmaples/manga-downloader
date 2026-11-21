// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows && !darwin

package trash

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// files moves the files into the home trash of the freedesktop.org Trash
// specification ($XDG_DATA_HOME/Trash, ~/.local/share/Trash), with the
// .trashinfo file desktops restore them from. A file on another filesystem
// than the trash can't be moved there and is left where it is.
func files(paths []string, failed map[string]error) {
	dir, err := trashDir()
	if err == nil {
		err = os.MkdirAll(filepath.Join(dir, "files"), 0o700)
	}
	if err == nil {
		err = os.MkdirAll(filepath.Join(dir, "info"), 0o700)
	}
	if err != nil {
		for _, p := range paths {
			failed[p] = err
		}
		return
	}
	for _, p := range paths {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			continue
		}
		dest := freeName(filepath.Join(dir, "files"), filepath.Base(p))
		name := filepath.Base(dest)
		info := filepath.Join(dir, "info", name+".trashinfo")
		abs, _ := filepath.Abs(p)
		u := (&url.URL{Path: abs}).EscapedPath()
		body := fmt.Sprintf("[Trash Info]\nPath=%s\nDeletionDate=%s\n", u, time.Now().Format("2006-01-02T15:04:05"))
		if err := os.WriteFile(info, []byte(body), 0o600); err != nil {
			failed[p] = err
			continue
		}
		if err := os.Rename(p, dest); err != nil {
			_ = os.Remove(info)
			failed[p] = err
		}
	}
}

// trashDir is the home trash
func trashDir() (string, error) {
	if d := os.Getenv("XDG_DATA_HOME"); strings.HasPrefix(d, "/") {
		return filepath.Join(d, "Trash"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "Trash"), nil
}
