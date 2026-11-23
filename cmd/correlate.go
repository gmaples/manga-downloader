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

// progressLines are what the command prints as the output folder's
// correlation goes (see core.CorrelateProgress): the files it looks at, the
// files read now and then, and each file moved to the trash (or not) as its
// batch is done
func progressLines(p core.CorrelateProgress) []line {
	switch p.Step {
	case core.CorrelateListed:
		if p.Total == 0 {
			return nil
		}
		return []line{{text: fmt.Sprintf("- looking for duplicates among %d %s", p.Total, plural(p.Total, "file", "files"))}}
	case core.CorrelateComparing:
		if p.Done%core.CorrelateLogEvery != 0 && p.Done != p.Total {
			return nil
		}
		return []line{{text: fmt.Sprintf("- compared %d of %d files that share a size", p.Done, p.Total)}}
	}
	var out []line
	for _, d := range p.Moved {
		if d.Err != nil {
			out = append(out, line{text: fmt.Sprintf("- couldn't remove duplicate %s (%s): %s", d.Name, why(d), d.Err), warn: true})
		} else {
			out = append(out, line{text: "- removed duplicate " + d.Name + ": " + why(d)})
		}
	}
	out = append(out, line{text: fmt.Sprintf("- moved %d of %d duplicates to the trash", p.Done, p.Total)})
	return out
}

// correlationLines are the total the command prints once the output folder's
// correlation is done (see core.Correlation), none when it moved nothing (a
// duplicate that couldn't go has its own line already)
func correlationLines(r *core.Correlation) []line {
	if r == nil || len(r.Removed) == 0 {
		return nil
	}
	if r.Permanent {
		return []line{{text: fmt.Sprintf("- deleted %s (%s) for good: this drive has no Recycle Bin", duplicates(len(r.Removed)), Bytes(r.RemovedBytes))}}
	}
	return []line{{text: fmt.Sprintf("- moved %d duplicate %s (%s) to the trash", len(r.Removed), plural(len(r.Removed), "file", "files"), Bytes(r.RemovedBytes))}}
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
