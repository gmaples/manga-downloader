// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"os"
	"strings"
)

// readLines calls each with every line of the file at path, named name in
// warnings, and its number (from 1). A missing file calls nothing; another
// read error is warned.
func readLines(path, name string, warn func(string, ...any), each func(line string, n int)) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			warn("%s: %s", name, err)
		}
		return
	}
	for i, line := range strings.Split(string(data), "\n") {
		each(line, i+1)
	}
}
