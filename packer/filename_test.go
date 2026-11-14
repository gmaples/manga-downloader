// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/elboletaire/manga-downloader/grabber"
)

func TestFilenameDefaultGroup(t *testing.T) {
	tests := []struct {
		name    string
		group   string
		version int
		want    string
	}{
		{"with group", "Alpha", 1, "Series 5 - Chapter 5 [Alpha]"},
		{"without group", "", 1, "Series 5 - Chapter 5"},
		{"duplicate", "Alpha", 2, "Series 5 - Chapter 5 [Alpha] v2"},
		{"duplicate without group", "", 2, "Series 5 - Chapter 5 v2"},
	}
	for _, tt := range tests {
		chapter := &grabber.Chapter{Number: 5, Title: "Chapter 5", Group: tt.group}
		parts := NewChapterFileTemplateParts("Series", chapter)
		parts.Version = tt.version
		got, err := NewFilenameFromTemplate(FilenameTemplateDefault, parts)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestVolumeTemplateDefaults(t *testing.T) {
	for _, templ := range []string{"", FilenameTemplateDefault, filenameTemplateLegacyDefault} {
		if got := VolumeTemplate(templ); got != FilenameVolumeTemplateDefault {
			t.Errorf("VolumeTemplate(%q) = %q, want the volume default", templ, got)
		}
	}

	parts := NewVolumeFileTemplateParts("Series", 1, "", []*grabber.Chapter{{Number: 1, Group: "Alpha"}})
	got, err := NewFilenameFromTemplate(VolumeTemplate(FilenameTemplateDefault), parts)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Series Vol. 01 (1) [Alpha]" {
		t.Errorf("volume with group: got %q", got)
	}
}

func TestMigrateFilenameTemplate(t *testing.T) {
	if got := MigrateFilenameTemplate(filenameTemplateLegacyDefault); got != FilenameTemplateDefault {
		t.Errorf("legacy default: got %q, want the new default", got)
	}
	custom := "{{.Series}} {{.Number}}"
	if got := MigrateFilenameTemplate(custom); got != custom {
		t.Errorf("custom template: got %q, want it unchanged", got)
	}
}

// a volume or bundle keeps a group only when all its chapters share it
func TestVolumeGroupIsShared(t *testing.T) {
	tests := []struct {
		name     string
		chapters []*grabber.Chapter
		want     string
	}{
		{"shared", []*grabber.Chapter{{Number: 1, Group: "Alpha"}, {Number: 2, Group: "Alpha"}}, "Alpha"},
		{"mixed", []*grabber.Chapter{{Number: 1, Group: "Alpha"}, {Number: 2, Group: "Delta"}}, ""},
	}
	for _, tt := range tests {
		parts := NewVolumeFileTemplateParts("Series", 1, "", tt.chapters)
		if parts.Group != tt.want {
			t.Errorf("%s: Group %q, want %q", tt.name, parts.Group, tt.want)
		}
	}
}

// TestFilenameLabelUnchanged checks that a chapter with a composed label is named,
// labelled and keyed as one carrying that label as its title, and that
// {{.SiteTitle}} is the site's title alone
func TestFilenameLabelUnchanged(t *testing.T) {
	tests := []struct {
		name      string
		old       string
		chapter   grabber.Chapter
		file      string
		siteTitle string
	}{
		{"no title", "Chapter 12", grabber.Chapter{Number: 12, Label: grabber.Label{Style: grabber.LabelNumbered}}, "Series 12 - Chapter 12", ""},
		{"decimal", "Chapter 10.5", grabber.Chapter{Number: 10.5, Label: grabber.Label{Style: grabber.LabelNumbered}}, "Series 10.5 - Chapter 10.5", ""},
		{"titled", "Dead End", grabber.Chapter{Title: "Dead End", Number: 36, Label: grabber.Label{Style: grabber.LabelNumbered}}, "Series 36 - Dead End", "Dead End"},
		{"site's chapter word", "Capítulo 7: Foo", grabber.Chapter{Title: "Capítulo 7: Foo", Number: 7, Label: grabber.Label{Style: grabber.LabelNumbered}}, "Series 7 - Capítulo 7; Foo", "Capítulo 7; Foo"},
		{"group", "Chapter 3", grabber.Chapter{Number: 3, Group: "Alpha", Label: grabber.Label{Style: grabber.LabelNumbered}}, "Series 3 - Chapter 3 [Alpha]", ""},
		{"prefixed", "Chapter 5 - Foo", grabber.Chapter{Title: "Foo", Number: 5, Label: grabber.Label{Style: grabber.LabelPrefixed}}, "Series 5 - Chapter 5 - Foo", "Foo"},
		{"number first", "Chapter 1 Market", grabber.Chapter{Title: "Market", Number: 1, Label: grabber.Label{Style: grabber.LabelNumberFirst}}, "Series 1 - Chapter 1 Market", "Market"},
		{"number first, no title", "Chapter 10.5", grabber.Chapter{Number: 10.5, Label: grabber.Label{Style: grabber.LabelNumberFirst}}, "Series 10.5 - Chapter 10.5", ""},
		{"site's number text", "Chapter 05", grabber.Chapter{Number: 5, Label: grabber.Label{Style: grabber.LabelNumbered, Number: "05"}}, "Series 5 - Chapter 05", ""},
		{"part", "Chapter 364 Part 2", grabber.Chapter{Number: 364, Label: grabber.Label{Style: grabber.LabelNumbered, Part: "2"}}, "Series 364 - Chapter 364 Part 2", ""},
		{"one-shot", "Chapter", grabber.Chapter{Number: 4, Label: grabber.Label{Style: grabber.LabelUnnumbered}}, "Series 4 - Chapter", ""},
		{"another language", "Capítulo 5", grabber.Chapter{Number: 5, Label: grabber.Label{Style: grabber.LabelNumbered, Lang: "es"}}, "Series 5 - Capítulo 5", ""},
		{"another language, numbered", "Chapitre 3", grabber.Chapter{Number: 3, Label: grabber.Label{Style: grabber.LabelNumbered, Lang: "fr"}}, "Series 3 - Chapitre 3", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := tt.chapter
			before.Title, before.Label = tt.old, grabber.Label{}
			name := func(c *grabber.Chapter) string {
				got, err := NewFilenameFromTemplate(FilenameTemplateDefault, NewChapterFileTemplateParts("Series", c))
				if err != nil {
					t.Fatal(err)
				}
				return got
			}
			after := tt.chapter
			if got, want := name(&after), name(&before); got != want || got != tt.file {
				t.Errorf("file name = %q, want %q (before: %q)", got, tt.file, want)
			}
			if got, want := chapterLabel(&after), chapterLabel(&before); got != want {
				t.Errorf("EPUB label = %q, want %q", got, want)
			}
			if got, want := chapterKey(&after), chapterKey(&before); got != want {
				t.Errorf("EPUB key = %q, want %q", got, want)
			}
			got, err := NewFilenameFromTemplate("{{.SiteTitle}}", NewChapterFileTemplateParts("Series", &after))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.siteTitle {
				t.Errorf("{{.SiteTitle}} = %q, want %q", got, tt.siteTitle)
			}
		})
	}
}

// oldPaddedTemplate names mangadex chapters as their padded labels did
const oldPaddedTemplate = `{{.Series}} {{.Number}} - Chapter {{pad .Number 4}}{{if .SiteTitle}} {{.SiteTitle}}{{end}}{{if .Group}} [{{.Group}}]{{end}}{{if gt .Version 1}} v{{.Version}}{{end}}`

// TestPadRecreatesPaddedNames checks that pad gives back the names mangadex and
// inmanga chapters got from their zero-padded labels
func TestPadRecreatesPaddedNames(t *testing.T) {
	inmanga := strings.Replace(oldPaddedTemplate, "Chapter", "Capítulo", 1)
	tests := []struct {
		templ   string
		chapter grabber.Chapter
		want    string
	}{
		{oldPaddedTemplate, grabber.Chapter{Title: "Market", Number: 1, Group: "Alpha", Label: grabber.Label{Style: grabber.LabelNumberFirst}}, "Series 1 - Chapter 0001 Market [Alpha]"},
		{oldPaddedTemplate, grabber.Chapter{Number: 5, Group: "O TRANSLATIONS", Label: grabber.Label{Style: grabber.LabelNumberFirst}}, "Series 5 - Chapter 0005 [O TRANSLATIONS]"},
		{oldPaddedTemplate, grabber.Chapter{Title: "La mare dels ous", Number: 1187, Label: grabber.Label{Style: grabber.LabelNumberFirst}}, "Series 1187 - Chapter 1187 La mare dels ous"},
		{inmanga, grabber.Chapter{Number: 999, Label: grabber.Label{Style: grabber.LabelNumbered, Lang: "es"}}, "Series 999 - Capítulo 0999"},
	}
	for _, tt := range tests {
		got, err := NewFilenameFromTemplate(tt.templ, NewChapterFileTemplateParts("Series", &tt.chapter))
		if err != nil {
			t.Fatal(err)
		}
		if got != tt.want {
			t.Errorf("file name = %q, want %q", got, tt.want)
		}
	}
}

func TestPadNumber(t *testing.T) {
	tests := []struct {
		number any
		width  int
		want   string
	}{
		{"5", 4, "0005"},
		{"0", 4, "0000"},
		{"10.5", 4, "0010.5"},
		{"12345", 4, "12345"},
		{"1234", 4, "1234"},
		{"1-7", 2, "01-7"},
		{3, 2, "03"},
		{"", 4, ""},
		{"5", 0, "5"},
	}
	for _, tt := range tests {
		if got := padNumber(tt.number, tt.width); got != tt.want {
			t.Errorf("padNumber(%v, %d) = %q, want %q", tt.number, tt.width, got, tt.want)
		}
	}
	got, err := NewFilenameFromTemplate("{{pad .Number 3}} {{pad .Volume 2}}", FilenameTemplateParts{Number: "7.5", Volume: 3})
	if err != nil || got != "007.5 03" {
		t.Errorf("template pad = %q (%v), want %q", got, err, "007.5 03")
	}
}

func TestBlankTemplateIsTheDefault(t *testing.T) {
	parts := FilenameTemplateParts{Series: "Series", Number: "5", Title: "Chapter 5", Version: 1}
	for _, templ := range []string{"", "  ", "\t\n"} {
		got, err := NewFilenameFromTemplate(templ, parts)
		if err != nil || got != "Series 5 - Chapter 5" {
			t.Errorf("NewFilenameFromTemplate(%q) = %q, %v", templ, got, err)
		}
		if VolumeTemplate(templ) != FilenameVolumeTemplateDefault {
			t.Errorf("VolumeTemplate(%q) is not the volume default", templ)
		}
	}
}

func TestTemplateWarning(t *testing.T) {
	for templ, warn := range map[string]bool{
		"":                              false,
		"  ":                            false,
		FilenameTemplateDefault:         false,
		"{{.Series}} {{.Title}}":        false,
		"{{.Series}} {{pad .Number 3}}": false,
		"{{.Series}} {{.SiteTitle}}":    false,
		"{{.Series}}":                   true,
		"fixed":                         true,
	} {
		if got := TemplateWarning(templ) != ""; got != warn {
			t.Errorf("TemplateWarning(%q) warns = %v, want %v", templ, got, warn)
		}
	}
}

func TestSanitizeFilenameWindowsCharacters(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// the replacements upstream already made are unchanged
		{`a/b\c:d?e"f`, `a_b_c;d¿e'f`},
		// the rest of the characters Windows refuses
		{"a<b>c|d*e", "a_b_c_d_e"},
		// control characters, tab and newline included, and DEL
		{"a\x00b\x1fc\td\ne\x7ff", "a_b_c_d_e_f"},
		// names without any of them stay byte for byte
		{"One Piece 0001 [Group]", "One Piece 0001 [Group]"},
		{"Ван-Пис 第1話 원피스", "Ван-Пис 第1話 원피스"},
		// trailing dots and spaces are left to SanitizeDirname
		{"Vol. 1...", "Vol. 1..."},
	}
	for _, tt := range tests {
		if got := SanitizeFilename(tt.in); got != tt.want {
			t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeDirname(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"One Piece", "One Piece"},
		{"Ван-Пис", "Ван-Пис"},
		{`What?: "Yes" <No> | *`, `What¿; 'Yes' _No_ _ _`},
		// trailing dots and spaces go, leading ones and inner ones stay
		{"The End. . ", "The End"},
		{" Dr. Stone", " Dr. Stone"},
		// nothing usable left
		{"", DirnameUntitled},
		{"   ", DirnameUntitled},
		{".", DirnameUntitled},
		{"..", DirnameUntitled},
		// separators can't climb or make it absolute
		{"../../etc", "_._.._etc"},
		{`..\..\Windows`, "_._.._Windows"},
		{"/abs/path", "_abs_path"},
		{`C:\Users`, "C;_Users"},
		// reserved device names, any case, with or without an extension
		{"CON", "CON_"},
		{"nul", "nul_"},
		{"Com1", "Com1_"},
		{"LPT9", "LPT9_"},
		{"CON.txt", "CON_.txt"},
		// a leading dot would hide the folder on Linux, macOS and Android
		{".hack//Sign", "_hack__Sign"},
		{"...Hidden", "_..Hidden"},
		// not reserved
		{"CONAN", "CONAN"},
		{"COM10", "COM10"},
		{"Console Wars", "Console Wars"},
	}
	for _, tt := range tests {
		if got := SanitizeDirname(tt.in, MaxNameBytes); got != tt.want {
			t.Errorf("SanitizeDirname(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSanitizeDirnameLength(t *testing.T) {
	// 100 CJK runes are 300 UTF-8 bytes: too long for ext4/f2fs
	long := strings.Repeat("話", 100)
	got := SanitizeDirname(long, 240)
	if len(got) > 240 || !utf8.ValidString(got) {
		t.Fatalf("len %d, valid %v", len(got), utf8.ValidString(got))
	}
	if !strings.Contains(got, "…") || !strings.HasPrefix(got, "話") || !strings.HasSuffix(got, "話") {
		t.Errorf("shortened to %q, want start…end", got)
	}
	if again := SanitizeDirname(long, 240); again != got {
		t.Errorf("not deterministic: %q then %q", got, again)
	}
	if short := SanitizeDirname("One Piece", 240); short != "One Piece" {
		t.Errorf("short name changed to %q", short)
	}
}

func TestOutputNameIsPortable(t *testing.T) {
	tests := []struct {
		format, in, want string
	}{
		// unchanged when already fine
		{FormatCBZ, "One Piece 1 - Chapter 1", "One Piece 1 - Chapter 1.cbz"},
		{FormatEPUB, "One Piece 1 - Chapter 1", "One Piece 1 - Chapter 1.epub"},
		// the template's own text and a bundle's title are sanitized too
		{FormatCBZ, "Re:Zero <1-10> | bundle", "Re;Zero _1-10_ _ bundle.cbz"},
		{FormatCBR, "a/b", "a_b.cbr"},
		// a dot before the extension is fine, but a raw folder can't end in one
		{FormatCBZ, "Dr. Stone Vol. 1.", "Dr. Stone Vol. 1..cbz"},
		{FormatRaw, "Dr. Stone Vol. 1. ", "Dr. Stone Vol. 1"},
		// hidden and reserved names
		{FormatCBZ, ".hack 1 - Chapter 1", "_hack 1 - Chapter 1.cbz"},
		{FormatCBZ, "CON", "CON_.cbz"},
		{FormatRaw, "nul", "nul_"},
	}
	for _, tt := range tests {
		if got := outputName(tt.format, tt.in); got != tt.want {
			t.Errorf("outputName(%q, %q) = %q, want %q", tt.format, tt.in, got, tt.want)
		}
	}
}

func TestOutputNameLengthKeepsTheVersion(t *testing.T) {
	long := strings.Repeat("長い題名", 30) // 360 bytes
	first := outputName(FormatCBZ, "Series 12 - "+long)
	second := outputName(FormatCBZ, "Series 12 - "+long+" v2")
	for _, name := range []string{first, second} {
		if len(name) > MaxNameBytes || !utf8.ValidString(name) || !strings.HasSuffix(name, ".cbz") {
			t.Fatalf("%q: %d bytes, valid %v", name, len(name), utf8.ValidString(name))
		}
		if !strings.HasPrefix(name, "Series 12 - ") {
			t.Errorf("%q lost the series and number", name)
		}
	}
	if !strings.HasSuffix(second, " v2.cbz") || first == second {
		t.Errorf("version suffix lost: %q vs %q", first, second)
	}
}
