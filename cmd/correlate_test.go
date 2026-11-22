// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
)

// the command prints the correlation as it goes: what it looks at, the reads
// now and then, and each file of a batch as it's moved
func TestProgressLines(t *testing.T) {
	text := func(ls []line) string {
		var out []string
		for _, l := range ls {
			s := l.text
			if l.warn {
				s = "!" + s
			}
			out = append(out, s)
		}
		return strings.Join(out, "|")
	}
	cases := []struct {
		p    core.CorrelateProgress
		want string
	}{
		{core.CorrelateProgress{Step: core.CorrelateListed, Done: 3, Total: 3}, "- looking for duplicates among 3 files"},
		{core.CorrelateProgress{Step: core.CorrelateListed}, ""},
		{core.CorrelateProgress{Step: core.CorrelateComparing, Done: 7, Total: 120}, ""},
		{core.CorrelateProgress{Step: core.CorrelateComparing, Done: 50, Total: 120}, "- compared 50 of 120 files that share a size"},
		{core.CorrelateProgress{Step: core.CorrelateComparing, Done: 120, Total: 120}, "- compared 120 of 120 files that share a size"},
		{core.CorrelateProgress{Step: core.CorrelateMoving, Done: 2, Total: 5, Moved: []core.Duplicate{
			{Name: "S 1 v2.cbz", Kept: "S 1.cbz", SameBytes: true},
			{Name: "S 2 - b.cbz", Kept: "S 2 - a.cbz", Number: "2", Err: errors.New("busy")},
		}}, "- removed duplicate S 1 v2.cbz: same file as S 1.cbz|!- couldn't remove duplicate S 2 - b.cbz (chapter 2 is already S 2 - a.cbz): busy|- moved 2 of 5 duplicates to the trash"},
	}
	for _, c := range cases {
		if got := text(progressLines(c.p)); got != c.want {
			t.Errorf("%+v:\n got %q\nwant %q", c.p, got, c.want)
		}
	}
}
