// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package trash

import (
	"errors"
	"fmt"
	"os"
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

// files sends the files to the Recycle Bin with SHFileOperationW and
// FOF_ALLOWUNDO, one call per file so one that fails doesn't stop the others.
// As in Explorer, a file on a drive with no Recycle Bin is deleted outright.
func files(paths []string, failed map[string]error) {
	for _, p := range paths {
		if err := recycle(p); err != nil {
			failed[p] = err
		}
	}
}

// recycle sends one file to the Recycle Bin
func recycle(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	from, err := syscall.UTF16FromString(path)
	if err != nil {
		return err
	}
	// pFrom is a list ending with two NULs
	from = append(from, 0)
	op := newFileOp(foDelete, &from[0], trashFlags)
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op[0])))
	runtime.KeepAlive(from)
	if r != 0 {
		return fmt.Errorf("couldn't move %s to the Recycle Bin (error 0x%x)", path, r)
	}
	if aborted(op) {
		return fmt.Errorf("moving %s to the Recycle Bin was cancelled", path)
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s is still there after moving it to the Recycle Bin", path)
	}
	return nil
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
