// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import "testing"

func TestNumberMatcher(t *testing.T) {
	m, ok := NewNumberMatcher("Гинтама", FilenameTemplateDefault, FormatCBZ)
	if !ok {
		t.Fatal("default template not matched")
	}
	cases := map[string]string{
		"Гинтама 141 - Chapter 141 В жизни нет очков сохранения [Sherlyn].cbz": "141",
		"Гинтама 141 - В жизни нет очков сохранения.cbz":                       "141",
		"Гинтама 140 - Пожалуйста v2.cbz":                                      "140",
		"Гинтама 13.1 - Экстра.CBZ":                                            "13.1",
		"Гинтама 0007 - padded.cbz":                                            "7",
		"Гинтама 1-50.cbz":                                                     "",
		"Гинтама Vol. 01 (1-10).cbz":                                           "",
		"Гинтама 141 - x.epub":                                                 "",
		"Гинтама 2 141 - x.cbz":                                                "",
		"Другая 141 - x.cbz":                                                   "",
		"Гинтама141 - x.cbz":                                                   "",
	}
	for name, want := range cases {
		got, ok := m.Number(name)
		if got != want || ok != (want != "") {
			t.Errorf("Number(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
}

func TestNumberMatcherDeclines(t *testing.T) {
	for _, tc := range []struct{ templ, format string }{
		{"{{.Series}} {{.Title}} {{.Number}}", FormatCBZ},
		{"{{.Series}} - {{.Title}}", FormatCBZ},
		{FilenameTemplateDefault, FormatRaw},
	} {
		if _, ok := NewNumberMatcher("S", tc.templ, tc.format); ok {
			t.Errorf("%q in %s matched", tc.templ, tc.format)
		}
	}
	if m, ok := NewNumberMatcher("S", "{{.Series}} c{{pad .Number 4}}", FormatEPUB); !ok {
		t.Error("padded template not matched")
	} else if n, _ := m.Number("S c0012.epub"); n != "12" {
		t.Errorf("padded number %q", n)
	}
}
