// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Each English template is its format with the verbs named, so the window
// shows the same words as the notice.
func TestNoticeTemplatesFollowFormats(t *testing.T) {
	for _, f := range noticeFormats {
		names := placeholderRE.FindAllStringSubmatch(catalog[f.key], -1)
		i := 0
		want := formatVerbRE.ReplaceAllStringFunc(f.format, func(string) string {
			name := names[i][1]
			i++
			return "{{" + name + "}}"
		})
		if catalog[f.key] != want {
			t.Errorf("%s: English text %q, want %q", f.key, catalog[f.key], want)
		}
	}
}

// joinedLiteralRE joins the parts of a string literal split over lines with +
var joinedLiteralRE = regexp.MustCompile(`"\s*\+\s*"`)

// Each format is still written somewhere in the code, so a notice reworded
// there doesn't leave a dead entry.
func TestNoticeFormatsAreInTheCode(t *testing.T) {
	var code strings.Builder
	for _, dir := range []string{"../browser", "../core", "../downloader", "../grabber", "../packer", "../ratelimit", "."} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") || filepath.Base(file) == "notices.go" {
				continue
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			code.WriteString(joinedLiteralRE.ReplaceAllString(string(raw), ""))
		}
	}
	src := code.String()
	// these formats carry one argument's literal value, written apart in the code
	literal := map[string]string{
		"notice.site.genericPlainPage":    "the plain page",
		"notice.site.genericChaptersPage": "the plain chapters page",
		"notice.site.genericBrowser":      "a hidden browser",
		"notice.mangaplus.fewUse":         "use --language to pick another",
		"notice.mangaplus.fewPick":        "pick another one with --language",
	}
	for _, f := range noticeFormats {
		format := f.format
		if value, ok := literal[f.key]; ok {
			if !strings.Contains(src, `"`+value+`"`) {
				t.Errorf("%s: %q is no longer in the code", f.key, value)
				continue
			}
			format = strings.Replace(format, value, "%s", 1)
		}
		if !strings.Contains(src, `"`+format+`"`) {
			t.Errorf("%s: %q is no longer in the code", f.key, format)
		}
	}
}

// A notice printed with sample values comes back as its key, with each value
// under its name, and renders the same English text.
func TestNoticeMessage(t *testing.T) {
	m, ok := noticeMessage(fmt.Sprintf("%s needs a real browser: starting it with a visible window (this can take a few seconds)...", "senkuro.com"))
	if !ok || m.Key != "notice.browser.startingVisible" || m.Args["host"] != "senkuro.com" {
		t.Fatalf("got %+v, %v", m, ok)
	}
	text := fmt.Sprintf("page %d: %s, trying again in %s (try %d of %d)", 5, "page 5 returned 404 Not Found: retry", "2s", 2, 3)
	m, ok = noticeMessage(text)
	if !ok || m.Key != "notice.download.retry" || m.Text != text {
		t.Fatalf("got %+v, %v", m, ok)
	}
	if got := render(m.Key, m.Args); got != text {
		t.Errorf("rendered %q, want %q", got, text)
	}
	if m.Args["page"] != "5" || m.Args["tries"] != "3" {
		t.Errorf("args %v", m.Args)
	}
	for _, f := range noticeFormats {
		verbs := formatVerbRE.FindAllString(f.format, -1)
		args := make([]any, len(verbs))
		for i, v := range verbs {
			if strings.HasSuffix(v, "d") {
				args[i] = 10 + i
			} else {
				args[i] = fmt.Sprintf("value %d: x", i)
			}
		}
		text := fmt.Sprintf(f.format, args...)
		m, ok := noticeMessage(text)
		if !ok || m.Key != f.key {
			t.Errorf("%s: %q matched %q, %v", f.key, text, m.Key, ok)
			continue
		}
		if got := render(m.Key, m.Args); got != text {
			t.Errorf("%s: rendered %q, want %q", f.key, got, text)
		}
	}
	if _, ok := noticeMessage("some error from a site"); ok {
		t.Error("an unknown notice matched")
	}
}
