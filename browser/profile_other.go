// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package browser

import (
	"os"
	"syscall"
)

// lockFile holds an exclusive lock on path until release is called or the
// process ends, failing at once when another process holds it
func lockFile(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
