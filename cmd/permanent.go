// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"golang.org/x/term"
)

// On a drive with no Recycle Bin (a memory card, a network share), moving a
// file to the trash would delete it for good. The command line asks first,
// y/N, at a terminal; with no terminal to ask at, it keeps the files.

// stdinTerminal and stdin are where the answer comes from, variables so
// tests can stand in for the terminal
var (
	stdinTerminal           = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
	stdin         io.Reader = os.Stdin
)

// askYes prints question with " [y/N] " and reads one line: only y or yes, in
// any case, is a yes. false without asking when stdin isn't a terminal.
func askYes(question string) bool {
	if !stdinTerminal() {
		return false
	}
	fmt.Fprint(color.Output, question+" [y/N] ")
	line, _ := bufio.NewReader(stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// duplicates writes a count of duplicate files
func duplicates(n int) string {
	return fmt.Sprintf("%d duplicate %s", n, plural(n, "file", "files"))
}

// ConfirmPermanent asks at the terminal, and keeps the files without one
func (p plainEvents) ConfirmPermanent(dir string, n int, bytes int64) bool {
	if stdinTerminal() {
		return askYes(color.RedString("- %s has no Recycle Bin: delete %s (%s) for good?", dir, duplicates(n), Bytes(bytes)))
	}
	yellow("- %s has no Recycle Bin: keeping %s", dir, duplicates(n))
	return false
}

// ConfirmPermanent keeps the files: a question can't be asked under the
// progress bars, which hold the terminal
func (e *bubbleEvents) ConfirmPermanent(dir string, n int, _ int64) bool {
	e.println(color.RedString("- %s has no Recycle Bin, so %s were kept. To be asked, run with the output piped (… | cat)", dir, duplicates(n)))
	return false
}
