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
//  2. filter, the language filter in force, unless it's blank, "all" or "any",
//     and only when the site lists languages (options isn't nil): a site
//     with no language choice ignores the filter, so it tells nothing
//  3. options, the site's language list, when it lists exactly one language
//  4. declared, the language the site's page declares (<html lang>), when
//     title is written in a script of that language (see declaredFits), as
//     some sites declare a template's default ("en") whatever they publish
//  5. the script of the chapter titles, then of title, only when the script
//     belongs to one language (see scriptLanguage)
//  6. "": no tag, as nothing tells the language with confidence
//
// Values that don't parse as a language tag ("English", "raw") are skipped,
// so they never end up in a folder name. Tags come out in their usual form
// ("pt-br" is "pt-BR").
func FolderLanguage(chapters grabber.Filterables, filter string, options *grabber.FilterOptions, title, declared string) string {
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

	if options != nil {
		switch strings.ToLower(strings.TrimSpace(filter)) {
		case "", grabber.FilterAll, "any":
		default:
			if tag, ok := languageTag(filter); ok {
				return tag
			}
		}
	}

	if options != nil && len(options.Values) == 1 {
		if tag, ok := languageTag(options.Values[0]); ok {
			return tag
		}
	}

	if tag, ok := languageTag(declared); ok && declaredFits(tag, title) {
		return tag
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

// declaredFits reports whether title is written in the script of the
// declared language tag: Cyrillic for ru, Latin for en, kana or Han for ja,
// and so on. A title with no letters of that script doesn't fit, so a
// template's lang="en" never tags a Cyrillic or Japanese title.
func declaredFits(tag, title string) bool {
	script, _ := language.Make(tag).Script()
	var tables []*unicode.RangeTable
	switch script.String() {
	case "Cyrl":
		tables = []*unicode.RangeTable{unicode.Cyrillic}
	case "Latn":
		tables = []*unicode.RangeTable{unicode.Latin}
	case "Jpan":
		tables = []*unicode.RangeTable{unicode.Hiragana, unicode.Katakana, unicode.Han}
	case "Kore":
		tables = []*unicode.RangeTable{unicode.Hangul}
	case "Hans", "Hant":
		tables = []*unicode.RangeTable{unicode.Han}
	default:
		return false
	}
	for _, r := range title {
		if unicode.In(r, tables...) {
			return true
		}
	}

	return false
}

// scriptLanguage returns the language of s when its letters belong to one
// language, "" otherwise: Ukrainian-only letters (Ї Є Ґ) are uk, Russian
// letters Ukrainian doesn't have (Ы Э Ъ) ru, kana ja and Hangul ko. Ы and Э are
// Belarusian too and Ъ Bulgarian: tagging those few titles ru is the price of
// tagging Russian ones. Other Cyrillic, Han (Chinese or Japanese kanji) and
// Latin text tell no single language, so they give no tag.
func scriptLanguage(s string) string {
	var ukrainian, russian, kana, hangul bool
	for _, r := range s {
		switch {
		case strings.ContainsRune("ЇїЄєҐґ", r):
			ukrainian = true
		case strings.ContainsRune("ЫыЭэЪъ", r):
			russian = true
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana = true
		case unicode.Is(unicode.Hangul, r):
			hangul = true
		}
	}
	switch {
	case ukrainian:
		return "uk"
	case russian:
		return "ru"
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

// WithSettings returns a copy of the series that reads settings, for a front
// end that downloads the series again with other settings while an earlier
// download of it, with its own, is still going
func (s *Series) WithSettings(settings *grabber.Settings) *Series {
	c := *s
	c.settings = settings

	return &c
}
