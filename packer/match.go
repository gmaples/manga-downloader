// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/elboletaire/manga-downloader/grabber"
)

// NumberText writes a chapter number the way {{.Number}} does in file names:
// 1, 10.5
func NumberText(n float64) string {
	return strings.Replace(fmt.Sprintf("%.1f", n), ".0", "", 1)
}

// numberSentinel stands for the chapter number when a template is rendered to
// find where the number goes
const numberSentinel = "987654321"

// titleSentinel stands for the chapter's title, which must not come before
// the number: the prefix would then differ from chapter to chapter
const titleSentinel = "TitleSentinel"

// numberRe is a chapter number at the start of a text: 141, 0141, 13.1
var numberRe = regexp.MustCompile(`^\d+(\.\d+)?`)

// versionRe is the " v2" of a second copy, ending the name
var versionRe = regexp.MustCompile(`^ v\d+$`)

// NumberMatcher reads the chapter number of a series' chapter files from
// their names, whatever their title or group: what the template writes
// before {{.Number}} (the series), the number, and the character the template
// writes after it up to the title. A file saved under another title, group or version of the
// same template is still recognised by its number.
type NumberMatcher struct {
	prefix, sep, ext string
}

// NewNumberMatcher returns the matcher of the chapter files a template names
// in format, false when the template doesn't name a chapter by its number
// (no {{.Number}}, or a title written before it) or the format is raw,
// whose chapters are folders.
func NewNumberMatcher(title, template, format string) (*NumberMatcher, bool) {
	if format == FormatRaw {
		return nil, false
	}
	parts := NewChapterFileTemplateParts(title, &grabber.Chapter{Title: titleSentinel})
	parts.Number = numberSentinel
	rendered, err := renderName(template, parts, 1)
	if err != nil {
		return nil, false
	}
	name := outputName(format, rendered)
	ext := name[strings.LastIndexByte(name, '.'):]
	stem := strings.TrimSuffix(name, ext)
	i := strings.Index(stem, numberSentinel)
	if i < 0 || strings.Contains(stem[:i], titleSentinel) {
		return nil, false
	}
	m := &NumberMatcher{prefix: stem[:i], ext: ext}
	// what comes between the number and the title (" - "), so another
	// series whose title goes on with a number ("Series 2 141 - …") isn't read
	// as this one's
	rest := stem[i+len(numberSentinel):]
	if j := strings.Index(rest, titleSentinel); j >= 0 {
		rest = rest[:j]
	}
	m.sep = rest
	return m, true
}

// Number returns the chapter number in a file's name, as NumberText writes
// it, false when the name isn't one of the series' chapter files
func (m *NumberMatcher) Number(name string) (string, bool) {
	if len(name) <= len(m.ext) || !strings.EqualFold(name[len(name)-len(m.ext):], m.ext) {
		return "", false
	}
	stem := name[:len(name)-len(m.ext)]
	if !strings.HasPrefix(stem, m.prefix) {
		return "", false
	}
	rest := stem[len(m.prefix):]
	num := numberRe.FindString(rest)
	if num == "" {
		return "", false
	}
	// a bundle's range ("1-50") or anything else glued to the number isn't a
	// chapter's; a " v2" copy of a file whose name ends with the number is
	after := rest[len(num):]
	switch {
	case after == "":
	case m.sep != "" && strings.HasPrefix(after, m.sep):
	case versionRe.MatchString(after):
	default:
		return "", false
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return "", false
	}
	return NumberText(n), true
}
