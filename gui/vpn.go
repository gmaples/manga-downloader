// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"github.com/elboletaire/manga-downloader/http"
	"github.com/elboletaire/manga-downloader/ratelimit"
)

// noteVPNLocked records what err says about a VPN for the site key: a sign of
// a block (http.VPNHint) sets the site's hint, a nil err (the site worked)
// clears it, and any other failure leaves it as it is. Called with c.mu held.
func (c *Controller) noteVPNLocked(key string, err error) {
	if key == "" {
		return
	}
	if err == nil {
		delete(c.vpn, key)
		return
	}
	if hint := http.VPNHint(err); hint != "" {
		if c.vpn == nil {
			c.vpn = map[string]string{}
		}
		c.vpn[key] = hint
	}
}

// VPNHint returns why a VPN might help with the site of url (http.VPNRegion,
// http.VPNDNS), "" when nothing the site answered says so
func (c *Controller) VPNHint(url string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vpn[ratelimit.SiteKey(url)]
}
