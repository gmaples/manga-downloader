// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package http

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"strings"
)

// The reasons VPNHint gives. Each only says a VPN *might* help: none of them
// proves the site blocks the network the request came from.
const (
	// VPNRegion is a 451, or a 403 page saying the site isn't available in
	// the country or region the request came from
	VPNRegion = "region"
	// VPNDNS is a site name that didn't resolve, or resolved to an address
	// nothing can be fetched from (127.0.0.1, 0.0.0.0): some networks block
	// sites that way
	VPNDNS = "dns"
)

// regionBodyLimit is how much of a 403 or 451 body is searched for a region
// block (a site's 403 can be a whole page, with the message far into it)
const regionBodyLimit = 64 << 10

// regionPhrases are what a page that refuses a country or region says,
// lowercased. They're whole phrases, as single words ("страна", "region")
// also turn up on ordinary error pages.
var regionPhrases = []string{
	"your country",
	"your region",
	"your location",
	"country or region",
	"in your area",
	"not available in",
	"only available in",
	"available only in",
	"geo-restrict",
	"georestrict",
	"geo-block",
	"geoblock",
	"только в странах",
	"в вашей стране",
	"в вашем регионе",
	"в странах снг",
	"お住まいの地域",
	"お住まいの国",
	"tu país",
	"tu región",
}

// regionBlocked reports whether a 403 body says the site refuses the
// request's country or region
func regionBlocked(body []byte) bool {
	lower := bytes.ToLower(body)
	for _, p := range regionPhrases {
		if bytes.Contains(lower, []byte(p)) {
			return true
		}
	}
	return false
}

// statusRegion reports whether a response with code and (the start of) body
// is a region block: any 451, or a 403 whose page says so
func statusRegion(code int, body []byte) bool {
	return code == http.StatusUnavailableForLegalReasons || (code == http.StatusForbidden && regionBlocked(body))
}

// VPNHint returns why a VPN might get err's request through (VPNRegion,
// VPNDNS), or "" when err is no sign of a block: an ordinary 4xx or 5xx, a
// 429, a timeout or a dropped connection, which an overloaded or broken site
// gives as well.
func VPNHint(err error) string {
	if err == nil {
		return ""
	}
	var se *StatusError
	if errors.As(err, &se) {
		if se.Region {
			return VPNRegion
		}
		return ""
	}
	var de *net.DNSError
	if errors.As(err, &de) && de.IsNotFound {
		return VPNDNS
	}
	var oe *net.OpError
	if errors.As(err, &oe) && oe.Op == "dial" && deadAddr(oe.Addr) {
		return VPNDNS
	}
	return ""
}

// deadAddr reports whether a dialled address is one a public site never has:
// a loopback or unspecified IP, the answer some networks' DNS gives for a
// blocked site
func deadAddr(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	host := addr.String()
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}
