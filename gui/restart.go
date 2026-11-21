// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

// Restart downloads again the unfinished chapters of group id: the one of
// its row key, or with key "" all of them (see restartTargetsLocked). They
// download as Download would download them selected alone, so a running
// download of the series takes them in its queue, and the selection becomes
// them. With the group's series loaded that happens now; otherwise url is
// returned with load set, and the load of url selects them and starts.
// Chapters download whole again: nothing of a failed try is kept.
func (c *Controller) Restart(id int, key string) (url string, load bool, err error) {
	c.mu.Lock()
	var g *group
	for _, cand := range c.groups {
		if cand.id == id {
			g = cand
		}
	}
	if g == nil {
		c.mu.Unlock()
		return "", false, uiErr("errors.noSuchGroup", nil, nil)
	}
	if c.loading {
		c.mu.Unlock()
		return "", false, errBusy
	}
	keys, labels := c.restartTargetsLocked(g, key)
	if len(keys)+len(labels) == 0 {
		c.mu.Unlock()
		return "", false, uiErr("errors.nothingToRestart", nil, nil)
	}
	if g.running {
		g.forgetLocked(keys)
	}
	c.resumeURL, c.resumeTodo, c.resumeKeys, c.resumeStart = g.url, labels, keys, true
	if c.series == nil || c.url != g.url || c.filtersChanged() {
		c.mu.Unlock()
		c.emit()
		return g.url, true, nil
	}
	c.applyResumeLocked(g.url)
	c.startPending = false
	c.mu.Unlock()
	c.emit()
	return g.url, false, c.Start()
}

// startRestart starts the chapters a load selected for Restart, logging
// a refusal, as nothing waits for it
func (c *Controller) startRestart() {
	err := c.Start()
	if err == nil {
		return
	}
	reason := err.Error()
	if m := messageOf(err); m != nil {
		reason = m.Text
	}
	c.mu.Lock()
	c.appendMsgLocked(LogError, msg("log.restartFailed", map[string]any{"reason": reason}))
	c.mu.Unlock()
	c.emit()
}

// restartTargetsLocked returns what Restart downloads again of group g, its
// row key's chapter or with key "" all: a restored group's chapters to do,
// by label, or another group's failed and cancelled chapters, by chapKey,
// leaving out those a newer download of the series took since. The caller
// holds c.mu.
func (c *Controller) restartTargetsLocked(g *group, key string) (keys, labels map[string]bool) {
	keys, labels = map[string]bool{}, map[string]bool{}
	if g.restored {
		for _, j := range g.jobs {
			if j.State == JobTodo && (key == "" || j.Key == key) {
				labels[j.Title] = true
			}
		}
		return keys, labels
	}
	newer := c.newerGroupsLocked(g)
	for _, chap := range g.chapters {
		k := chapKey(chap)
		if key != "" && k != key {
			continue
		}
		if m := g.markOf(k); m != MarkFailed && m != MarkCancelled {
			continue
		}
		taken := false
		for _, n := range newer {
			taken = taken || n.keys[k]
		}
		if !taken {
			keys[k] = true
		}
	}
	return keys, labels
}

// newerGroupsLocked returns the groups of g's series that started after it.
// The caller holds c.mu.
func (c *Controller) newerGroupsLocked(g *group) []*group {
	var out []*group
	after := false
	for _, cand := range c.groups {
		if cand == g {
			after = true
			continue
		}
		if after && !cand.restored && cand.url == g.url {
			out = append(out, cand)
		}
	}
	return out
}

// restartInfoLocked fills st, group g's state, with what Restart would
// download again: Unfinished, and Restart on the rows. The caller holds c.mu.
func (c *Controller) restartInfoLocked(g *group, st *GroupState) {
	keys, labels := c.restartTargetsLocked(g, "")
	st.Unfinished = len(keys) + len(labels)
	for i := range st.Jobs {
		j := &st.Jobs[i]
		switch {
		case j.Key == "" || j.Volume:
		case g.restored:
			j.Restart = j.State == JobTodo
		default:
			j.Restart = j.State == JobFailed && keys[j.Key]
		}
	}
}

// forgetLocked takes chapters keys out of running group g's run, so they
// can join its queue again: their failed rows and failures go, and its
// counts no longer hold them. The caller holds c.mu.
func (g *group) forgetLocked(keys map[string]bool) {
	labels := map[string]bool{}
	kept := g.chapters[:0]
	for _, chap := range g.chapters {
		if keys[chapKey(chap)] {
			labels[g.releaseLabel(chap)] = true
			continue
		}
		kept = append(kept, chap)
	}
	g.chapters = kept
	jobs := g.jobs[:0]
	for _, j := range g.jobs {
		if j.Key != "" && keys[j.Key] && !j.Volume {
			continue
		}
		jobs = append(jobs, j)
	}
	g.jobs = jobs
	failures := g.stats.failures[:0]
	for _, f := range g.stats.failures {
		if !labels[f.Label] {
			failures = append(failures, f)
		}
	}
	g.stats.failures = failures
	for k := range keys {
		delete(g.keys, k)
		delete(g.started, k)
		delete(g.failed, k)
		delete(g.done, k)
	}
	g.runinfo.Chapters -= len(keys)
	if !g.runinfo.Bundle && !g.runinfo.Volumes {
		g.runinfo.Rows -= len(keys)
	}
}
