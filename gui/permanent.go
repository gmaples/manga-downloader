// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

// On a drive with no Recycle Bin (a memory card, a network share), the
// automatic dedupe would delete duplicate files for good. It asks the window
// first: the group's download waits on the question until the user answers,
// or the group is cancelled, which answers no. One question is open at a time.

// PermanentDTO is the question the window shows: delete Files duplicate
// files (Bytes in all) in Dir for good, for group Group's download
type PermanentDTO struct {
	ID    int    `json:"id"`
	Group int    `json:"group"`
	Dir   string `json:"dir"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// permanentState is the Controller's open question, guarded by its mu
type permanentState struct {
	lastID  int
	pending *permanentAsk
}

// permanentAsk is the open question and where its answer goes
type permanentAsk struct {
	PermanentDTO
	answer chan bool
}

// ConfirmPermanent asks the window, and waits for the answer: false when the
// group is cancelled first
func (e events) ConfirmPermanent(dir string, n int, bytes int64) bool {
	c, g := e.c, e.g
	c.mu.Lock()
	c.permanent.lastID++
	ask := &permanentAsk{
		PermanentDTO: PermanentDTO{ID: c.permanent.lastID, Group: g.id, Dir: dir, Files: n, Bytes: bytes},
		answer:       make(chan bool, 1),
	}
	c.permanent.pending = ask
	stop := g.stop
	c.mu.Unlock()
	c.emit()

	yes := false
	select {
	case yes = <-ask.answer:
	case <-stop:
	}

	c.mu.Lock()
	if c.permanent.pending == ask {
		c.permanent.pending = nil
	}
	if yes {
		g.forGood = true
	}
	c.mu.Unlock()
	c.emit()
	return yes
}

// AnswerPermanent answers the open question id: yes deletes for good
func (c *Controller) AnswerPermanent(id int, yes bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ask := c.permanent.pending
	if ask == nil || ask.ID != id {
		return uiErr("errors.noSuchQuestion", nil, nil)
	}
	c.permanent.pending = nil
	ask.answer <- yes
	return nil
}

// PendingPermanent is the open question, nil when there's none
func (c *Controller) PendingPermanent() *PermanentDTO {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.permanent.pending == nil {
		return nil
	}
	p := c.permanent.pending.PermanentDTO
	return &p
}
