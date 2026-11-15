// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

// attach adds a group to c, as Start does, for tests that drive the events by
// hand, and returns it with its events
func attach(c *Controller) (*group, events) {
	g := newGroup()
	c.mu.Lock()
	c.lastID++
	g.id = c.lastID
	c.groups = append(c.groups, g)
	c.mu.Unlock()
	return g, events{c, g}
}

// lastGroup is the newest group, the zero GroupState when there's none
func lastGroup(c *Controller) GroupState {
	groups := c.Groups()
	if len(groups) == 0 {
		return GroupState{}
	}
	return groups[len(groups)-1]
}

// jobsOf, summaryOf and runOf are the newest group's rows, summary and run
func jobsOf(c *Controller) []Job       { return lastGroup(c).Jobs }
func summaryOf(c *Controller) *Summary { return lastGroup(c).Summary }
func runOf(c *Controller) RunInfo      { return lastGroup(c).Run }
