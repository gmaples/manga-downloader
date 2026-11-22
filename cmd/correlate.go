// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"fmt"

	"github.com/elboletaire/manga-downloader/core"
)

// line is one line the command prints, warn when it reports a problem
type line struct {
	text string
	warn bool
}

// correlationLines are what the command prints about the output folder's
// duplicates (see core.Correlation): a line per file moved to the trash,
// one per file that couldn't be, and a total
func correlationLines(r *core.Correlation) []line {
	if r == nil || len(r.Removed)+len(r.Failed) == 0 {
		return nil
	}
	var out []line
	for _, d := range r.Removed {
		out = append(out, line{text: "- removed duplicate " + d.Name + ": " + why(d)})
	}
	for _, d := range r.Failed {
		out = append(out, line{text: fmt.Sprintf("- couldn't remove duplicate %s (%s): %s", d.Name, why(d), d.Err), warn: true})
	}
	out = append(out, line{text: fmt.Sprintf("- moved %d duplicate %s (%s) to the trash", len(r.Removed), plural(len(r.Removed), "file", "files"), Bytes(r.RemovedBytes))})
	return out
}

// why says what a duplicate was matched to
func why(d core.Duplicate) string {
	if d.SameBytes {
		return "same file as " + d.Kept
	}
	return "chapter " + d.Number + " is already " + d.Kept
}

// plural picks the word for n
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Bytes writes a size the way people read it: 912 KB, 1.4 GB
func Bytes(n int64) string {
	const unit = 1000
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "kMGTPE"[exp])
}
