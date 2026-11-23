// Package filelock is an OS file lock that the system lets go of when the
// program stops, so a crash never leaves one behind
package filelock

import (
	"golang.org/x/sys/windows"
)

// TryLock holds path open with no sharing until release is called or the
// process ends, failing at once when another holder has it open
func TryLock(path string) (release func(), err error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
