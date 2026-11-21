// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MirrorsFileName is the file, next to the rate limits file in the download
// root, that names other servers to try for a page a server fails. Each line
// is "host: other host, other host", tried in that order with the same path
// and query, for example
//
//	img2.imglib.info: img3.cdnlibs.org, img3.cdnrulib.link
//
// Lines starting with # are comments. The downloader only reads it: nothing
// is built in, and a host the file doesn't list has no fallback.
const MirrorsFileName = "site-mirrors.txt"

// parseMirrorsLine reads one line: ok is false for a blank line or a comment
func parseMirrorsLine(line string) (host string, mirrors []string, ok bool, err error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, false, nil
	}
	// ": " ends the host, so one with a port (127.0.0.1:8080) can be listed
	k, v, found := strings.Cut(line, ": ")
	if !found {
		k, v, found = strings.Cut(line, ":")
	}
	if !found {
		return "", nil, false, fmt.Errorf("%q isn't \"host: other host, other host\"", line)
	}
	host = mirrorHost(k)
	if host == "" {
		return "", nil, false, fmt.Errorf("%q has no host", line)
	}
	for _, m := range strings.Split(v, ",") {
		m = mirrorHost(m)
		if m == "" {
			continue
		}
		if m == host {
			return "", nil, false, fmt.Errorf("%q names %s as its own fallback", line, host)
		}
		mirrors = append(mirrors, m)
	}
	if len(mirrors) == 0 {
		return "", nil, false, fmt.Errorf("%q names no other host", line)
	}
	return host, mirrors, true, nil
}

// mirrorHost is a host as the file writes it: trimmed and lower case. A
// pasted address ("https://img3.cdnlibs.org/") is cut to its host.
func mirrorHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(strings.TrimPrefix(s, "https://"), "http://")
	s, _, _ = strings.Cut(s, "/")
	return s
}

// readMirrors reads the mirrors file in the book's folder
func (b *Book) readMirrors() {
	path := filepath.Join(filepath.Dir(b.path), MirrorsFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			b.warn("%s: %s", MirrorsFileName, err)
		}
		return
	}
	for i, line := range strings.Split(string(data), "\n") {
		host, mirrors, ok, err := parseMirrorsLine(line)
		if err != nil {
			b.warn("%s line %d: %s (ignored)", MirrorsFileName, i+1, err)
			continue
		}
		if ok {
			if _, seen := b.mirrors[host]; !seen {
				b.mirrors[host] = mirrors
			}
		}
	}
}

// Mirrors returns the other hosts site-mirrors.txt names for host (the
// host of a page's address, with its port if it has one), in the file's
// order, nil when it names none
func (s *Site) Mirrors(host string) []string {
	b := s.book
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.mirrors[strings.ToLower(host)]
}
