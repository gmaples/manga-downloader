// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/language"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// LanguageMultiple tags a run whose chapters name more than one language
const LanguageMultiple = "mul"

// SeriesDir is the folder a run with series folders saves into: one folder
// named after title (packer.SanitizeDirname, shortened so the whole name fits
// packer.MaxNameBytes) inside root, with " [lang]" appended when lang isn't empty. root is used as given, so the folder picker
// still picks the parent and reruns reuse the same folder.
func SeriesDir(root, title, lang string) string {
	suffix := ""
	if lang != "" {
		suffix = " [" + lang + "]"
	}
	name := packer.SanitizeDirname(title, packer.MaxNameBytes-len(suffix)) + suffix

	return filepath.Join(root, name)
}

// languaged is a listed chapter that says its language (grabber.Chapter)
type languaged interface {
	GetLanguage() string
}

// FolderLanguage resolves the language tag of a run's folder, "" for none, first match wins:
//
//  1. the chapters' own languages: one known language is its tag, more than
//     one is LanguageMultiple
//  2. filter, the language filter in force, unless it's blank, "all" or "any"
//  3. options, the site's language list, when it lists exactly one language
//  4. the script of the chapter titles, then of title, only when the script
//     belongs to one language (see scriptLanguage)
//  5. "": no tag, as nothing tells the language with confidence
//
// Values that don't parse as a language tag ("English", "raw") are skipped,
// so they never end up in a folder name. Tags come out in their usual form
// ("pt-br" is "pt-BR").
func FolderLanguage(chapters grabber.Filterables, filter string, options *grabber.FilterOptions, title string) string {
	seen := map[string]bool{}
	var found []string
	for _, chap := range chapters {
		l, ok := chap.(languaged)
		if !ok {
			continue
		}
		if tag, ok := languageTag(l.GetLanguage()); ok && !seen[tag] {
			seen[tag] = true
			found = append(found, tag)
		}
	}
	switch {
	case len(found) == 1:
		return found[0]
	case len(found) > 1:
		return LanguageMultiple
	}

	switch strings.ToLower(strings.TrimSpace(filter)) {
	case "", grabber.FilterAll, "any":
	default:
		if tag, ok := languageTag(filter); ok {
			return tag
		}
	}

	if options != nil && len(options.Values) == 1 {
		if tag, ok := languageTag(options.Values[0]); ok {
			return tag
		}
	}

	var titles strings.Builder
	for _, chap := range chapters {
		titles.WriteString(chap.GetTitle())
		titles.WriteByte(' ')
	}
	if tag := scriptLanguage(titles.String()); tag != "" {
		return tag
	}
	if tag := scriptLanguage(title); tag != "" {
		return tag
	}

	return ""
}

// languageTag returns v as a language tag in its usual form, and false when v
// isn't one, or is "und"
func languageTag(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	tag, err := language.Parse(v)
	if err != nil {
		return "", false
	}
	s := tag.String()
	if s == "und" {
		return "", false
	}

	return s, true
}

// scriptLanguage returns the language of s when its script belongs to one
// language only, "" otherwise: Ukrainian-only letters (Ї Є Ґ) are uk, kana ja
// and Hangul ko. Cyrillic without those letters (Russian, Ukrainian without
// them, Belarusian, Bulgarian, …), Han (Chinese or Japanese kanji) and Latin
// text tell no single language, so they give no tag.
func scriptLanguage(s string) string {
	var ukrainian, kana, hangul bool
	for _, r := range s {
		switch {
		case strings.ContainsRune("ЇїЄєҐґ", r):
			ukrainian = true
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana = true
		case unicode.Is(unicode.Hangul, r):
			hangul = true
		}
	}
	switch {
	case ukrainian:
		return "uk"
	case kana:
		return "ja"
	case hangul:
		return "ko"
	}

	return ""
}

// OutputDir is the folder the series' Download saves into
func (s *Series) OutputDir() string {
	if s.settings == nil {
		return ""
	}

	return s.settings.OutputDir
}

// WithOutputDir returns a copy of the series that downloads into dir, with
// its own copy of the settings, so the settings the series was opened with,
// and the window that shows them, keep the folder the user picked. The
// site-rate-limits file stays in the folder picked, the download root.
func (s *Series) WithOutputDir(dir string) *Series {
	c := *s
	settings := grabber.Settings{}
	if s.settings != nil {
		settings = *s.settings
	}
	if c.limitsDir == "" {
		c.limitsDir = settings.OutputDir
	}
	settings.OutputDir = dir
	c.settings = &settings

	return &c
}
