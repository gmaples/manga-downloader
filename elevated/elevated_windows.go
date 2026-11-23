// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package elevated

import "golang.org/x/sys/windows"

// is asks the process token whether it's elevated (run as administrator)
func is() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}
