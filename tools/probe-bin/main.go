//go:build windows

// probe-bin prints what Windows reports about the Recycle Bin of each drive
// given on the command line. It is a fork-only dev tool: its output is the
// evidence for trash.HasRecycleBin's rule (cleanup spec, step 1a).
//
// Usage: probe-bin.exe C:\ J:\
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	shQueryRecycleBinW = shell32.NewProc("SHQueryRecycleBinW")
)

// shQueryRBInfo mirrors SHQUERYRBINFO. The real struct may be packed (20
// bytes) or naturally aligned (24 bytes); the buffer is 24 bytes and cbSize is
// set to each value in turn, so both answers are printed.
type shQueryRBInfo struct {
	cbSize      uint32
	_           uint32
	i64Size     int64
	i64NumItems int64
}

var driveTypes = map[uint32]string{
	0: "unknown", 1: "no root dir", 2: "removable", 3: "fixed",
	4: "network", 5: "cdrom", 6: "ramdisk",
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, `usage: probe-bin.exe C:\ J:\`)
		os.Exit(2)
	}
	for _, arg := range os.Args[1:] {
		root := filepath.VolumeName(arg) + `\`
		fmt.Printf("== %s (root %s)\n", arg, root)
		r, err := windows.UTF16PtrFromString(root)
		if err != nil {
			fmt.Printf("   bad path: %v\n", err)
			continue
		}
		dt := windows.GetDriveType(r)
		fmt.Printf("   GetDriveTypeW = %d (%s)\n", dt, driveTypes[dt])
		for _, size := range []uint32{20, 24} {
			var info shQueryRBInfo
			info.cbSize = size
			hr, _, _ := shQueryRecycleBinW.Call(uintptr(unsafe.Pointer(r)), uintptr(unsafe.Pointer(&info)))
			fmt.Printf("   SHQueryRecycleBinW cbSize=%d -> HRESULT 0x%08X", size, uint32(hr))
			if uint32(hr) == 0 {
				fmt.Printf(" (S_OK) items=%d bytes=%d", info.i64NumItems, info.i64Size)
			}
			fmt.Println()
		}
	}
}
