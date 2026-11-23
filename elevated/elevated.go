// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Package elevated tells whether the program runs with administrator rights:
// an elevated process on Windows, root elsewhere.
package elevated

// Is reports whether the program runs elevated
func Is() bool {
	return is()
}
