// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
)

// terminalWith makes stdin a terminal (or not) holding input, for the test
func terminalWith(t *testing.T, terminal bool, input string) {
	t.Helper()
	wasT, wasIn := stdinTerminal, stdin
	stdinTerminal = func() bool { return terminal }
	stdin = strings.NewReader(input)
	t.Cleanup(func() { stdinTerminal, stdin = wasT, wasIn })
}

// only y or yes, in any case, deletes for good; anything else, an empty
// line or no terminal at all keeps the files
func TestConfirmPermanent(t *testing.T) {
	for _, tc := range []struct {
		terminal bool
		input    string
		want     bool
	}{
		{true, "y\n", true},
		{true, "YES\n", true},
		{true, " Yes \r\n", true},
		{true, "\n", false},
		{true, "n\n", false},
		{true, "yep\n", false},
		{true, "", false},
		{false, "y\n", false},
	} {
		terminalWith(t, tc.terminal, tc.input)
		if got := (plainEvents{}).ConfirmPermanent("J:\\Manga\\X", 2, 6_400_000); got != tc.want {
			t.Errorf("terminal %v, %q: %v, want %v", tc.terminal, tc.input, got, tc.want)
		}
	}
}

// the total says when the duplicates went for good, and nothing about moving
// when none could go
func TestCorrelationLinesPermanent(t *testing.T) {
	r := &core.Correlation{Removed: []core.Duplicate{{Name: "a"}, {Name: "b"}}, RemovedBytes: 6_400_000, Permanent: true}
	if got := correlationLines(r); len(got) != 1 || got[0].text != "- deleted 2 duplicate files (6.4 MB) for good: this drive has no Recycle Bin" {
		t.Errorf("permanent: %+v", got)
	}
	r = &core.Correlation{Failed: []core.Duplicate{{Name: "a"}}}
	if got := correlationLines(r); len(got) != 0 {
		t.Errorf("none moved: %+v", got)
	}
}
