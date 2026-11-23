// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/elboletaire/manga-downloader/core"
	"github.com/elboletaire/manga-downloader/elevated"
)

// --delete-series takes the series' chapter files out of the output folder:
// into "[Empty Me]/<series>" in it, for the user to empty, or with --force
// (elevated only) to the Recycle Bin or trash, as the app's Delete series does

var (
	deleteSeries bool
	forceDelete  bool
)

// checkForce refuses --force without administrator rights, and without
// --delete-series, before anything is fetched
func checkForce() error {
	if !forceDelete {
		return nil
	}
	if !deleteSeries {
		return errors.New("--force only goes with --delete-series")
	}
	if !elevated.Is() {
		return errors.New("--force needs administrator rights (an elevated prompt on Windows, root elsewhere); without it, --delete-series moves the files to " + core.EmptyMeDir)
	}
	return nil
}

// runDeleteSeries deletes the series' files from the output folder and
// returns the exit code: 0 when everything went, 1 otherwise
func runDeleteSeries(out io.Writer, series *core.Series, root string, force bool) int {
	plan, err := series.DeletePlan(root, false)
	if errors.Is(err, core.ErrNothingToDelete) {
		fmt.Fprintf(out, "- no file of %s in %s, nothing to delete\n", series.Title, root)
		return 0
	}
	if err != nil {
		fmt.Fprintf(out, "Error: %s\n", err)
		return 1
	}
	to, where := "", "the trash"
	if !force {
		to = core.EmptyMeFolder(root, series.Title)
		where = to
	}
	fmt.Fprintf(out, "- deleting %s: %s (%s) to %s\n", series.Title, files(len(plan.Names)), Bytes(plan.Bytes), where)
	r := plan.Run(to, func(p core.DeleteProgress) {
		fmt.Fprintf(out, "- moved %d of %d\n", p.Done, p.Total)
	})
	failed := make([]string, 0, len(r.Failed))
	for n := range r.Failed {
		failed = append(failed, n)
	}
	sort.Strings(failed)
	for _, n := range failed {
		fmt.Fprintf(out, "- couldn't move %s: %s\n", n, r.Failed[n])
	}
	if len(failed) > 0 {
		fmt.Fprintf(out, "- moved %s (%s) of %s to %s, %d couldn't be moved\n", files(len(r.Moved)), Bytes(r.Bytes), series.Title, where, len(failed))
		return 1
	}
	fmt.Fprintf(out, "Deleted series %s from %s (%s, %s, moved to %s)\n", series.Title, root, files(len(r.Moved)), Bytes(r.Bytes), where)
	return 0
}

// files writes a count of files
func files(n int) string {
	return fmt.Sprintf("%d %s", n, plural(n, "file", "files"))
}
