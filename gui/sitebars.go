// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"time"

	"github.com/elboletaire/manga-downloader/ratelimit"
)

// SiteBar is the progress of one site that's downloading: the first chapter
// in flight of its running groups (Title and Tags, Done of Total, Total 0
// while its page list is fetched, Title "" between chapters), Until, the
// end of its pause after a 429, zero when it isn't paused, and Parallel, how
// many of its requests may be under way at once as learned from its speed
// (see ratelimit.Parallel)
type SiteBar struct {
	Site        string
	Title, Tags string
	Done, Total int64
	Until       time.Time
	Parallel    int
	// VPN is why a VPN might help with the site, "" when nothing says so
	// (see Controller.VPNHint)
	VPN string
}

// SiteBars returns a bar for each site a running group downloads from, in the
// order their groups started. Groups from one site share its bar, which shows
// the chapter of theirs that started first.
func (c *Controller) SiteBars() []SiteBar {
	c.mu.Lock()
	var bars []SiteBar
	first := map[string]*flight{}
	index := map[string]int{}
	for _, g := range c.groups {
		if !g.running {
			continue
		}
		if _, ok := index[g.siteKey]; !ok {
			index[g.siteKey] = len(bars)
			bars = append(bars, SiteBar{Site: g.siteKey})
		}
		for _, f := range g.flights {
			if cur := first[g.siteKey]; cur == nil || f.seq < cur.seq {
				first[g.siteKey] = f
			}
		}
		for _, f := range g.flightOf {
			if cur := first[g.siteKey]; cur == nil || f.seq < cur.seq {
				first[g.siteKey] = f
			}
		}
	}
	for key, f := range first {
		b := &bars[index[key]]
		b.Title, b.Tags, b.Done, b.Total = f.title, f.tags, f.done, f.total
	}
	for i := range bars {
		bars[i].VPN = c.vpn[bars[i].Site]
	}
	c.mu.Unlock()
	// read outside c.mu: the pacing has locks of its own
	for i := range bars {
		bars[i].Until = ratelimit.Cooling(bars[i].Site)
		bars[i].Parallel = ratelimit.Parallel(bars[i].Site)
	}
	return bars
}
