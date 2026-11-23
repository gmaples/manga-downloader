//go:build !windows

// Package filelock is an OS file lock that the system lets go of when the
// program stops, so a crash never leaves one behind
package filelock

import (
	"os"
	"syscall"
)

// TryLock holds an exclusive lock on path until release is called or the
// process ends, failing at once when another holder has it
func TryLock(path string) (release func(), err error) {
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
