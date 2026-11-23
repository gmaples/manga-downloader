// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package trash

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// SHFileOperationW's operation and flags (shellapi.h)
const (
	foDelete          = 0x0003
	fofSilent         = 0x0004
	fofNoConfirmation = 0x0010
	fofAllowUndo      = 0x0040
	fofNoErrorUI      = 0x0400
	fofNoConfirmMkdir = 0x0200
	trashFlags        = fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofNoConfirmMkdir
)

var procSHFileOperationW = syscall.NewLazyDLL("shell32.dll").NewProc("SHFileOperationW")

// osHasRecycleBin tells whether path's drive has a Recycle Bin: a fixed
// drive whose bin SHQueryRecycleBinW can read. Checked on Greg's PC
// (tools/probe-bin): C: is fixed (3) and the query gives S_OK; a removable
// exFAT card is 2 and the query fails with E_FAIL. The query takes
// SHQUERYRBINFO at its 64-bit size, 24 bytes: 20 is refused (E_INVALIDARG).
func osHasRecycleBin(p string) bool {
	root, err := syscall.UTF16PtrFromString(filepath.VolumeName(path(p)) + `\`)
	if err != nil {
		return false
	}
	if r, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(root))); r != driveFixed {
		return false
	}
	var info struct {
		cbSize      uint32
		_           uint32
		i64Size     int64
		i64NumItems int64
	}
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, _ := procSHQueryRecycleBinW.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&info)))
	return r == 0
}

// driveFixed is GetDriveTypeW's DRIVE_FIXED
const driveFixed = 3

var (
	procGetDriveTypeW      = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDriveTypeW")
	procSHQueryRecycleBinW = syscall.NewLazyDLL("shell32.dll").NewProc("SHQueryRecycleBinW")
)

// files sends the files to the Recycle Bin with one SHFileOperationW call
// and FOF_ALLOWUNDO, then looks at which are still there: those failed.
// Files only passes files on a drive with a Recycle Bin, since on one
// without it this call deletes them for good, as Explorer does.
func files(paths []string, failed map[string]error) {
	var from []uint16
	var present []string
	for _, p := range paths {
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			continue
		}
		u, err := syscall.UTF16FromString(path(p))
		if err != nil {
			failed[p] = err
			continue
		}
		// each path ends with its NUL, and the list with one more
		from = append(from, u...)
		present = append(present, p)
	}
	if len(present) == 0 {
		return
	}
	from = append(from, 0)
	op := newFileOp(foDelete, &from[0], trashFlags)
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op[0])))
	runtime.KeepAlive(from)
	var callErr error
	switch {
	case r != 0:
		callErr = fmt.Errorf("the Recycle Bin refused it (error 0x%x)", r)
	case aborted(op):
		callErr = errors.New("moving to the Recycle Bin was cancelled")
	default:
		callErr = errors.New("it's still there after moving it to the Recycle Bin")
	}
	for _, p := range present {
		if _, err := os.Lstat(p); err == nil {
			failed[p] = callErr
		}
	}
}

// path is p as SHFileOperationW takes it: a full path
func path(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// SHFILEOPSTRUCTW is packed to 1 byte on 32-bit Windows and to 8 on 64-bit,
// so it's laid out by hand: hwnd, wFunc, pFrom, pTo, fFlags,
// fAnyOperationsAborted, hNameMappings, lpszProgressTitle
const ptr = unsafe.Sizeof(uintptr(0))

// abortedAt is where fAnyOperationsAborted is
func abortedAt() uintptr {
	if ptr == 8 {
		// hwnd 8, wFunc 4 + 4, pFrom 8, pTo 8, fFlags 2 + 2
		return 36
	}
	// hwnd 4, wFunc 4, pFrom 4, pTo 4, fFlags 2
	return 18
}

// newFileOp is a SHFILEOPSTRUCTW for operation fn on from with flags
func newFileOp(fn uint32, from *uint16, flags uint16) []byte {
	size := uintptr(30)
	if ptr == 8 {
		size = 56
	}
	op := make([]byte, size)
	*(*uint32)(unsafe.Pointer(&op[ptr])) = fn
	if ptr == 8 {
		*(*uintptr)(unsafe.Pointer(&op[16])) = uintptr(unsafe.Pointer(from))
		*(*uint16)(unsafe.Pointer(&op[32])) = flags
	} else {
		*(*uint32)(unsafe.Pointer(&op[8])) = uint32(uintptr(unsafe.Pointer(from)))
		*(*uint16)(unsafe.Pointer(&op[16])) = flags
	}
	return op
}

// aborted reads fAnyOperationsAborted
func aborted(op []byte) bool {
	return *(*int32)(unsafe.Pointer(&op[abortedAt()])) != 0
}
