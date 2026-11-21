// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRequestMarksRegionBlocks checks which answers the request marks as a
// region block: a 451 always, a 403 only when its page says so (even far into
// a big page), never another status
func TestRequestMarksRegionBlocks(t *testing.T) {
	shortenRetries(t)
	// remanga.org's own wording (2026-11-21), after 40 KB of page
	padding := strings.Repeat("<div class=x></div>", 40<<10/19)
	cases := []struct {
		name   string
		code   int
		body   string
		region bool
	}{
		{"remanga's CIS-only 403", 403, padding + "<p>Сайт доступен только в странах СНГ</p>", true},
		{"Cloudflare 1009", 403, "The owner of this website has banned the country or region your IP address is in.", true},
		{"451", 451, "", true},
		{"plain 403", 403, "<h1>403 Forbidden</h1>", false},
		{"hotlink 403", 403, "<title>Error 403</title>", false},
		{"a 403 page with страница", 403, "Страница не найдена", false},
		{"400 saying a country", 400, "not available in your country", false},
		{"404", 404, "not found", false},
		{"429", 429, "slow down", false},
		{"503", 503, "your region is busy", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			_, err := Get(RequestParams{URL: srv.URL, DisableRetry: true})
			var se *StatusError
			if !errors.As(err, &se) || se.Code != tc.code {
				t.Fatalf("err = %v, want a %d StatusError", err, tc.code)
			}
			if se.Region != tc.region {
				t.Errorf("Region = %v, want %v", se.Region, tc.region)
			}
			want := ""
			if tc.region {
				want = VPNRegion
			}
			// wrapped the way a page or chapter error reaches the app
			if got := VPNHint(fmt.Errorf("download: page 1: %w", err)); got != want {
				t.Errorf("VPNHint = %q, want %q", got, want)
			}
		})
	}
}

// TestVPNHintDNS checks a name that doesn't resolve, and one that resolves to
// a dead address, give VPNDNS, and that other network failures give nothing
func TestVPNHintDNS(t *testing.T) {
	loop := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}
	zero := &net.TCPAddr{IP: net.IPv4zero, Port: 443}
	public := &net.TCPAddr{IP: net.ParseIP("93.184.216.34"), Port: 443}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"no such host", &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "desu.me", IsNotFound: true}}, VPNDNS},
		{"refused on 127.0.0.1", &net.OpError{Op: "dial", Net: "tcp", Addr: loop, Err: errors.New("connection refused")}, VPNDNS},
		{"refused on 0.0.0.0", &net.OpError{Op: "dial", Net: "tcp", Addr: zero, Err: errors.New("connection refused")}, VPNDNS},
		{"refused on a public address", &net.OpError{Op: "dial", Net: "tcp", Addr: public, Err: errors.New("connection refused")}, ""},
		{"DNS server timeout", &net.DNSError{Err: "i/o timeout", Name: "x", IsTimeout: true}, ""},
		{"reset while reading", &net.OpError{Op: "read", Net: "tcp", Addr: loop, Err: errors.New("connection reset")}, ""},
		{"timeout", fmt.Errorf("timed out: %w", context.DeadlineExceeded), ""},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		if got := VPNHint(fmt.Errorf("Get: %w", tc.err)); tc.err != nil && got != tc.want {
			t.Errorf("%s: VPNHint = %q, want %q", tc.name, got, tc.want)
		}
	}
	if got := VPNHint(nil); got != "" {
		t.Errorf("VPNHint(nil) = %q", got)
	}
}

// TestRequestUnresolvableHost checks a real lookup of a name that can't exist
// gives VPNDNS through the request's own error
func TestRequestUnresolvableHost(t *testing.T) {
	_, err := Get(RequestParams{URL: "http://no-such-host.invalid/", DisableRetry: true})
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := VPNHint(err); got != VPNDNS {
		t.Skipf("this network doesn't answer NXDOMAIN for .invalid (err %v); VPNHint = %q", err, got)
	}
}
