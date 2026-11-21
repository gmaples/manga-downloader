// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"golang.org/x/sys/windows"
)

// lockFile holds path open with no sharing until release is called or the
// process ends, failing at once when another process has it open
func lockFile(path string) (release func(), err error) {
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
