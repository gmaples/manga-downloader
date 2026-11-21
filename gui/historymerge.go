// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"reflect"
	"sort"
)

// Two copies of the app running at once both keep history.txt. A save used to
// write this copy's history over the file, so the other copy's series and
// chapters since this one read it were lost. Now a save reads the file first
// and merges (mergeHistory): a series this copy changed since its last save
// (or the load) is written as this copy has it, and every other series as the
// file has it.

// copyHistory returns a deep copy of entries
func copyHistory(entries []HistoryEntry) []HistoryEntry {
	out := make([]HistoryEntry, len(entries))
	for i, e := range entries {
		e.Chapters = append([]HistoryChapter(nil), e.Chapters...)
		out[i] = e
	}
	return out
}

// mergeHistory merges this copy's history (ours) with the file's (disk),
// given the history as this copy last loaded or saved it (base). By series URL:
//   - one ours changed or added since base is ours;
//   - one ours has as in base is the file's, as another copy may have changed
//     it, or left out when the file no longer has it (another copy cleared it);
//   - one ours dropped since base (a clear, or the cap) stays dropped, unless
//     the file's differs from base (another copy changed it since);
//   - one only the file has (another copy added it) is the file's.
//
// The result is newest first by When, at most historyMax entries.
func mergeHistory(base, ours, disk []HistoryEntry) []HistoryEntry {
	byURL := func(entries []HistoryEntry) map[string]HistoryEntry {
		m := make(map[string]HistoryEntry, len(entries))
		for _, e := range entries {
			if _, dup := m[e.URL]; !dup {
				m[e.URL] = e
			}
		}
		return m
	}
	b, o, d := byURL(base), byURL(ours), byURL(disk)
	var out []HistoryEntry
	seen := map[string]bool{}
	for _, e := range ours {
		if seen[e.URL] {
			continue
		}
		seen[e.URL] = true
		was, inBase := b[e.URL]
		if !inBase || !reflect.DeepEqual(was, e) {
			out = append(out, e)
			continue
		}
		if f, ok := d[e.URL]; ok {
			out = append(out, f)
		}
	}
	for _, f := range disk {
		if seen[f.URL] {
			continue
		}
		seen[f.URL] = true
		was, inBase := b[f.URL]
		if _, kept := o[f.URL]; inBase && !kept && reflect.DeepEqual(was, f) {
			continue
		}
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].When.After(out[j].When) })
	if len(out) > historyMax {
		out = out[:historyMax]
	}
	return copyHistory(out)
}
