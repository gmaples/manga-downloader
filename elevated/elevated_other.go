// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows

package elevated

import "os"

// is reports whether the process runs as root
func is() bool {
	return os.Geteuid() == 0
}
