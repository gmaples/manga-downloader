// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// a long title is shortened so the folder name, tag included, fits 255 bytes
func TestSeriesDirLongTitle(t *testing.T) {
	name := filepath.Base(SeriesDir("root", strings.Repeat("話", 100), "zh-Hant"))
	if len(name) > packer.MaxNameBytes || !strings.HasSuffix(name, " [zh-Hant]") {
		t.Errorf("%q: %d bytes", name, len(name))
	}
}

func TestSeriesDir(t *testing.T) {
	root := filepath.Join("lib", "Manga")
	tests := []struct {
		title, lang, want string
	}{
		{"One Piece", "", filepath.Join(root, "One Piece")},
		{"One Piece", "en", filepath.Join(root, "One Piece [en]")},
		{"Ван-Пис", "ru", filepath.Join(root, "Ван-Пис [ru]")},
		{"Re:Zero?", "mul", filepath.Join(root, "Re;Zero¿ [mul]")},
		{"", "unk", filepath.Join(root, "Untitled [unk]")},
		{"../../escape", "", filepath.Join(root, "_._.._escape")},
		{"CON", "ja", filepath.Join(root, "CON_ [ja]")},
		{".hack//Sign", "ja", filepath.Join(root, "_hack__Sign [ja]")},
	}
	for _, tt := range tests {
		got := SeriesDir(root, tt.title, tt.lang)
		if got != tt.want {
			t.Errorf("SeriesDir(%q, %q, %q) = %q, want %q", root, tt.title, tt.lang, got, tt.want)
		}
		if filepath.Dir(got) != root {
			t.Errorf("SeriesDir(%q, %q) = %q is not one folder inside the root", tt.title, tt.lang, got)
		}
	}
}

// chaptersIn lists chapters with the given languages and titles, in pairs
func chaptersIn(langTitle ...string) grabber.Filterables {
	var out grabber.Filterables
	for i := 0; i+1 < len(langTitle); i += 2 {
		out = append(out, &grabber.Chapter{Language: langTitle[i], Title: langTitle[i+1], Number: float64(i/2 + 1)})
	}
	return out
}

func TestFolderLanguage(t *testing.T) {
	one := &grabber.FilterOptions{Values: []string{"es"}}
	two := &grabber.FilterOptions{Values: []string{"es", "en"}}
	tests := []struct {
		name     string
		chapters grabber.Filterables
		filter   string
		options  *grabber.FilterOptions
		title    string
		want     string
	}{
		// 1. the chapters' own languages
		{"one chapter language", chaptersIn("en", "Chapter 1", "en", "Chapter 2"), "", nil, "", "en"},
		{"codes normalised", chaptersIn("pt-br", "Capítulo 1"), "", nil, "", "pt-BR"},
		{"case ignored for sameness", chaptersIn("EN", "a", "en", "b"), "", nil, "", "en"},
		{"several languages", chaptersIn("en", "a", "es", "b"), "", nil, "", LanguageMultiple},
		{"chapter language beats the filter", chaptersIn("en", "a"), "es", one, "Ван-Пис", "en"},
		{"unusable chapter values skipped", chaptersIn("English", "a", "xx", "b", "ja", "c"), "", nil, "", "ja"},
		// 2. the filter
		{"filter", chaptersIn("", "Chapter 1"), "fr", nil, "", "fr"},
		{"filter all ignored", chaptersIn("", "Chapter 1"), "all", nil, "", LanguageUnknown},
		{"filter any ignored", chaptersIn("", "Chapter 1"), "ANY", nil, "", LanguageUnknown},
		{"unusable filter ignored", chaptersIn("", "Chapter 1"), "Español", one, "", "es"},
		// 3. a site that lists one language
		{"single site language", chaptersIn("", "Chapter 1"), "", one, "", "es"},
		{"two site languages tell nothing", chaptersIn("", "Chapter 1"), "", two, "", LanguageUnknown},
		// 4. scripts, chapter titles before the series title
		{"ukrainian letters", chaptersIn("", "Розділ 1: Їжак"), "", nil, "", "uk"},
		{"kana", chaptersIn("", "第1話 はじまり"), "", nil, "", "ja"},
		{"hangul", chaptersIn("", "제1화"), "", nil, "", "ko"},
		{"other cyrillic", chaptersIn("", "Глава 1"), "", nil, "", "ru"},
		{"han only", chaptersIn("", "第一章"), "", nil, "", "zh"},
		{"kana beats han and cyrillic", chaptersIn("", "第一章 Глава の"), "", nil, "", "ja"},
		{"ukrainian beats kana", chaptersIn("", "の ї"), "", nil, "", "uk"},
		{"chapter titles before the series title", chaptersIn("", "Глава 1"), "", nil, "第一章", "ru"},
		{"series title when chapters say nothing", chaptersIn("", "Chapter 1"), "", nil, "Ван-Пис", "ru"},
		// 5. nothing tells
		{"latin is not english", chaptersIn("", "Chapter 1"), "", nil, "One Piece", LanguageUnknown},
		{"nothing at all", nil, "", nil, "", LanguageUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FolderLanguage(tt.chapters, tt.filter, tt.options, tt.title)
			if got != tt.want {
				t.Errorf("FolderLanguage = %q, want %q", got, tt.want)
			}
		})
	}
}

// WithOutputDir leaves the original series and its settings alone
func TestWithOutputDirCopiesSettings(t *testing.T) {
	settings := &grabber.Settings{OutputDir: "root", Format: "cbz"}
	series := &Series{Title: "T", settings: settings}

	moved := series.WithOutputDir(filepath.Join("root", "T [en]"))

	if settings.OutputDir != "root" || series.OutputDir() != "root" {
		t.Errorf("original changed: settings %q, series %q", settings.OutputDir, series.OutputDir())
	}
	if moved.OutputDir() != filepath.Join("root", "T [en]") {
		t.Errorf("moved.OutputDir() = %q", moved.OutputDir())
	}
	if moved.settings.Format != "cbz" || moved.Title != "T" {
		t.Errorf("copy lost the rest: %+v %q", moved.settings, moved.Title)
	}
	if (&Series{}).OutputDir() != "" {
		t.Error("a series without settings has no output dir")
	}
}

// cbzIn lists the archives in dir by reading it, as filepath.Glob would read
// the "[en]" of a series folder as a character class
func cbzIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".cbz") {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}

// a run into a series folder saves there, a rerun with skip-existing finds
// the files there, and nothing lands in the root
func TestDownloadIntoSeriesFolderAndRerun(t *testing.T) {
	site := newFakeSite(t,
		fakeChapter{number: 1, pages: 3},
		fakeChapter{number: 2, pages: 1},
	)
	series, root := newSeries(t, site, false)
	site.Settings.SkipExisting = true
	dir := SeriesDir(root, series.Title, "en")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	if _, err := series.WithOutputDir(dir).Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("Download: %v", err)
	}
	files := cbzIn(t, dir)
	want := []string{
		filepath.Join(dir, "Test Series 1 - Chapter 1.cbz"),
		filepath.Join(dir, "Test Series 2 - Chapter 2.cbz"),
	}
	if !slices.Equal(files, want) {
		t.Fatalf("files in the series folder = %q, want %q", files, want)
	}
	if got := cbzIn(t, root); len(got) != 0 {
		t.Errorf("files written to the root: %q", got)
	}
	// complete archive, pages in order
	entries := zipEntries(t, want[0])
	var pages []string
	for _, e := range entries {
		if strings.HasSuffix(e, ".png") || strings.HasSuffix(e, ".jpg") {
			pages = append(pages, e)
		}
	}
	if len(pages) != 3 || !slices.IsSorted(pages) {
		t.Errorf("chapter 1 pages = %q, want 3 in order", pages)
	}
	if site.Settings.OutputDir != root {
		t.Errorf("the site's settings moved to %q", site.Settings.OutputDir)
	}

	// the rerun reuses the folder and skips both chapters
	before := site.served[1] + site.served[2]
	rec = &recorder{}
	if _, err := series.WithOutputDir(dir).Download(context.Background(), listed(site), nil, "", rec); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	wantEvents := "exists 1 Test Series 1 - Chapter 1.cbz|exists 2 Test Series 2 - Chapter 2.cbz"
	if got := strings.Join(rec.events, "|"); got != wantEvents {
		t.Errorf("rerun events:\n got %q\nwant %q", got, wantEvents)
	}
	if after := site.served[1] + site.served[2]; after != before {
		t.Errorf("rerun fetched %d more chapters, want none", after-before)
	}
	if files := cbzIn(t, dir); !slices.Equal(files, want) {
		t.Errorf("rerun changed the folder: %q", files)
	}
}

// the archivers don't make folders, so the folder has to exist first
func TestDownloadIntoMissingFolderFails(t *testing.T) {
	site := newFakeSite(t, fakeChapter{number: 1, pages: 1})
	series, root := newSeries(t, site, false)
	dir := SeriesDir(root, series.Title, "")
	rec := &recorder{}

	_, _ = series.WithOutputDir(dir).Download(context.Background(), listed(site), nil, "", rec)

	if _, err := os.Stat(filepath.Join(dir, "Test Series 1 - Chapter 1.cbz")); err == nil {
		t.Fatal("packed into a folder nobody created; the controller's MkdirAll would be redundant")
	}
	if !strings.Contains(strings.Join(rec.events, "|"), "failed") {
		t.Errorf("no failure reported: %q", rec.events)
	}
}
