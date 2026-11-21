// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/http"
)

// regionErr is a 403 whose page said the site isn't available in the
// request's region, wrapped as a page error reaches the events
var regionErr = fmt.Errorf("download: page 1: %w", &http.StatusError{Code: 403, Region: true})

// The site bar gets a VPN hint from a failure that's a sign of a block, not
// from an ordinary one, and loses it once a chapter of the site downloads
func TestSiteBarVPNHint(t *testing.T) {
	c := NewController(&grabber.Settings{}, nil)
	g, e := attach(c)
	c.mu.Lock()
	g.running, g.siteKey = true, "example.com"
	c.mu.Unlock()
	vpn := func() string {
		t.Helper()
		bars := c.SiteBars()
		if len(bars) != 1 {
			t.Fatalf("bars = %+v, want one", bars)
		}
		return bars[0].VPN
	}

	// mangalib's "received 400" (a page missing on its main server) is no sign
	e.DownloadFailed(&grabber.Chapter{Number: 1}, fmt.Errorf("download: page 1: %w", &http.StatusError{Code: 400}))
	e.DownloadFailed(&grabber.Chapter{Number: 2}, fmt.Errorf("download: page 1: %w", &http.StatusError{Code: 403}))
	e.FetchFailed(&grabber.Chapter{Number: 3}, &http.StatusError{Code: 429})
	if got := vpn(); got != "" {
		t.Fatalf("VPN after a 400, a plain 403 and a 429 = %q, want none", got)
	}

	e.DownloadFailed(&grabber.Chapter{Number: 4}, regionErr)
	if got := vpn(); got != http.VPNRegion {
		t.Fatalf("VPN after a region 403 = %q, want %q", got, http.VPNRegion)
	}
	// an ordinary failure after it leaves the hint
	e.DownloadFailed(&grabber.Chapter{Number: 5}, &http.StatusError{Code: 500})
	if got := vpn(); got != http.VPNRegion {
		t.Errorf("VPN after a later 500 = %q, want it kept", got)
	}
	if got := c.VPNHint("https://www.example.com/manga/x"); got != http.VPNRegion {
		t.Errorf("VPNHint of the site's URL = %q, want %q", got, http.VPNRegion)
	}

	// a chapter that downloads clears it
	e.Downloaded(&grabber.Chapter{Number: 6})
	if got := vpn(); got != "" {
		t.Errorf("VPN after a chapter downloaded = %q, want none", got)
	}

	dnsErr := &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "example.com", IsNotFound: true}}
	e.FetchFailed(&grabber.Chapter{Number: 7}, fmt.Errorf("Get: %w", dnsErr))
	if got := vpn(); got != http.VPNDNS {
		t.Errorf("VPN after a name that didn't resolve = %q, want %q", got, http.VPNDNS)
	}
	// another site's hint stays apart
	if got := c.VPNHint("https://other.example"); got != "" {
		t.Errorf("another site's VPNHint = %q, want none", got)
	}
}

// A load that fails with a sign of a block gives its site a hint, and a load
// of the site that works clears it
func TestLoadVPNHint(t *testing.T) {
	const url = "https://example.com/manga/test"
	c, _, _ := newTestController(t, false, nil, chapters(1, 1)...)
	works := c.identify
	c.identify = func(string, *grabber.Settings) (grabber.Site, []error) {
		return nil, []error{regionErr}
	}
	if err := c.Load(url); err == nil {
		t.Fatal("the load worked")
	}
	if got := c.VPNHint(url); got != http.VPNRegion {
		t.Fatalf("VPNHint after a region 403 load = %q, want %q", got, http.VPNRegion)
	}

	// an ordinary failure leaves it
	c.identify = func(string, *grabber.Settings) (grabber.Site, []error) {
		return nil, []error{errors.New("site down")}
	}
	c.Load(url)
	if got := c.VPNHint(url); got != http.VPNRegion {
		t.Errorf("VPNHint after an ordinary failed load = %q, want it kept", got)
	}

	c.identify = works
	if err := c.Load(url); err != nil {
		t.Fatal(err)
	}
	if got := c.VPNHint(url); got != "" {
		t.Errorf("VPNHint after a load that worked = %q, want none", got)
	}
}
