// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package ratelimit

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseMirrorsLine(t *testing.T) {
	cases := []struct {
		line    string
		host    string
		mirrors []string
		ok, bad bool
	}{
		{line: "", ok: false},
		{line: "# img2.imglib.info: img3.cdnlibs.org", ok: false},
		{line: "img2.imglib.info: img3.cdnlibs.org, img3.cdnrulib.link", host: "img2.imglib.info", mirrors: []string{"img3.cdnlibs.org", "img3.cdnrulib.link"}, ok: true},
		{line: "  IMG2.imglib.info :https://img3.cdnlibs.org/ ,\r", host: "img2.imglib.info", mirrors: []string{"img3.cdnlibs.org"}, ok: true},
		{line: "127.0.0.1:8080: 127.0.0.1:9090", host: "127.0.0.1:8080", mirrors: []string{"127.0.0.1:9090"}, ok: true},
		{line: "img2.imglib.info:img3.cdnlibs.org", host: "img2.imglib.info", mirrors: []string{"img3.cdnlibs.org"}, ok: true},
		{line: "img2.imglib.info", bad: true},
		{line: "img2.imglib.info:", bad: true},
		{line: ": img3.cdnlibs.org", bad: true},
		{line: "img2.imglib.info: img2.imglib.info", bad: true},
	}
	for _, c := range cases {
		host, mirrors, ok, err := parseMirrorsLine(c.line)
		if (err != nil) != c.bad {
			t.Errorf("%q: err = %v, want error %v", c.line, err, c.bad)
			continue
		}
		if c.bad {
			continue
		}
		if ok != c.ok || host != c.host || !reflect.DeepEqual(mirrors, c.mirrors) {
			t.Errorf("%q = %q %q %v, want %q %q %v", c.line, host, mirrors, ok, c.host, c.mirrors, c.ok)
		}
	}
}

// The book reads site-mirrors.txt next to the rate limits file, warns about a
// line it can't read, keeps the first line of a host, and never writes it
func TestBookMirrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MirrorsFileName)
	text := "img2.imglib.info: img3.cdnlibs.org, img3.cdnrulib.link\nnonsense\nimg2.imglib.info: other.example\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	var warned []string
	b := Open(dir, func(f string, a ...any) { warned = append(warned, f) }, nil)
	s := b.Site("mangalib.org")
	if got := s.Mirrors("IMG2.imglib.info"); !reflect.DeepEqual(got, []string{"img3.cdnlibs.org", "img3.cdnrulib.link"}) {
		t.Errorf("Mirrors = %q", got)
	}
	if got := s.Mirrors("img3.cdnlibs.org"); got != nil {
		t.Errorf("Mirrors of an unlisted host = %q, want none", got)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "line %d") {
		t.Errorf("warnings = %q, want one for the bad line", warned)
	}
	b.saveParallel()
	if data, _ := os.ReadFile(path); string(data) != text {
		t.Errorf("the file changed:\n%s", data)
	}
	// no file: no mirrors, no warning
	warned = nil
	if got := Open(t.TempDir(), func(f string, a ...any) { warned = append(warned, f) }, nil).Site("x").Mirrors("img2.imglib.info"); got != nil || warned != nil {
		t.Errorf("without a file: %q, warnings %q", got, warned)
	}
}
