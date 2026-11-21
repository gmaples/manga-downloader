// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
)

// cbzNames lists the archive names in dir, "" entries for none
func cbzNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".cbz") {
			out = append(out, e.Name())
		}
	}
	return out
}

// runTwoChapters loads the fake series, applies fn to the settings, selects
// both chapters and runs them to the end
func runTwoChapters(t *testing.T, c *Controller, fn func(*grabber.Settings)) *Summary {
	t.Helper()
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(fn); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1-2"); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return waitFinished(t, c)
}

var twoChapters = []string{"Test Series 1 - Chapter 1.cbz", "Test Series 2 - Chapter 2.cbz"}

// with Series Folders off nothing changes: the files go to the picked folder
func TestSeriesFoldersOffKeepsThePickedFolder(t *testing.T) {
	c, _, dir := newTestController(t, false, nil, chapters(2, 1)...)

	s := runTwoChapters(t, c, func(s *grabber.Settings) { s.GuessLanguage = true })

	if s.Dir != dir {
		t.Errorf("summary dir %q, want %q", s.Dir, dir)
	}
	if got := cbzNames(t, dir); !slices.Equal(got, twoChapters) {
		t.Errorf("files %q, want %q", got, twoChapters)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("made a folder %q with Series Folders off", e.Name())
		}
	}
	if d := c.Destination(); d != dir {
		t.Errorf("Destination() = %q, want %q", d, dir)
	}
}

func TestSeriesFoldersSaveIntoTheSeriesFolder(t *testing.T) {
	tests := []struct {
		name  string
		guess bool
		want  string
	}{
		{"without language", false, "Test Series"},
		// the fake listing names no language and its titles are Latin, so
		// nothing tells the language with confidence: no tag
		{"with language, none known", true, "Test Series"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _, root := newTestController(t, false, nil, chapters(2, 1)...)
			want := filepath.Join(root, tt.want)

			s := runTwoChapters(t, c, func(s *grabber.Settings) {
				s.SeriesFolders = true
				s.GuessLanguage = tt.guess
			})

			if s.Dir != want {
				t.Errorf("summary dir %q, want %q", s.Dir, want)
			}
			if got := cbzNames(t, want); !slices.Equal(got, twoChapters) {
				t.Errorf("files in the series folder %q, want %q", got, twoChapters)
			}
			if got := cbzNames(t, root); len(got) != 0 {
				t.Errorf("files in the root: %q", got)
			}
			// the option keeps the picked folder, so a rerun doesn't nest
			if got := c.Settings().OutputDir; got != root {
				t.Errorf("OutputDir changed to %q", got)
			}
			if d := c.Destination(); d != want {
				t.Errorf("Destination() = %q, want %q", d, want)
			}
		})
	}
}

// a rerun reuses the folder, and skip-existing finds the files in it
func TestSeriesFoldersRerunSkipsExisting(t *testing.T) {
	c, _, root := newTestController(t, false, nil, chapters(2, 1)...)
	want := filepath.Join(root, "Test Series")
	on := func(s *grabber.Settings) {
		s.SeriesFolders, s.GuessLanguage, s.SkipExisting = true, true, true
	}
	runTwoChapters(t, c, on)

	s := runTwoChapters(t, c, on)

	if s.Dir != want || s.Existing != 2 || s.Saved != 0 {
		t.Errorf("rerun: dir %q existing %d saved %d; want %q, 2, 0", s.Dir, s.Existing, s.Saved, want)
	}
	entries, _ := os.ReadDir(want)
	if len(entries) != 2 {
		t.Errorf("series folder holds %d entries after the rerun, want 2 (no nested folder, no copies)", len(entries))
	}
}

// a known language tags the folder: here the language filter, set before the
// load, on a site that offers languages
func TestGuessLanguageTagsFromTheFilter(t *testing.T) {
	c, base, root := newTestController(t, false, nil, chapters(2, 1)...)
	site := &languageSite{fakeSite: base, options: grabber.FilterOptions{Values: []string{"en", "es"}, All: true}}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	if err := c.Configure(func(s *grabber.Settings) {
		s.SeriesFolders, s.GuessLanguage, s.Language = true, true, "en"
	}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Test Series [en]")

	s := runTwoChapters(t, c, func(*grabber.Settings) {})

	if s.Dir != want {
		t.Errorf("summary dir %q, want %q", s.Dir, want)
	}
	if got := cbzNames(t, want); !slices.Equal(got, twoChapters) {
		t.Errorf("files %q, want %q", got, twoChapters)
	}
}

// a site with no language choice ignores the filter, so the filter tells
// nothing: the saved "en" no longer tags a Russian series [en]
func TestGuessLanguageIgnoresTheFilterOfASiteWithoutLanguages(t *testing.T) {
	c, _, root := newTestController(t, false, nil, chapters(2, 1)...)
	if err := c.Configure(func(s *grabber.Settings) {
		s.SeriesFolders, s.GuessLanguage, s.Language = true, true, "en"
	}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Test Series")

	s := runTwoChapters(t, c, func(*grabber.Settings) {})

	if s.Dir != want {
		t.Errorf("summary dir %q, want %q", s.Dir, want)
	}
}

// declaringSite is a site whose series page declares its language
type declaringSite struct {
	*fakeSite
	lang string
}

func (d *declaringSite) PageLanguage() string { return d.lang }

// the language the site's page declares tags the folder when the title is
// written in its script, ahead of the saved filter the site ignores
func TestGuessLanguageTagsFromTheDeclaredLanguage(t *testing.T) {
	c, base, root := newTestController(t, false, nil, chapters(2, 1)...)
	base.title = "Мир Лидейл"
	site := &declaringSite{fakeSite: base, lang: "ru"}
	c.identify = func(_ string, s *grabber.Settings) (grabber.Site, []error) {
		site.Settings = s
		return site, nil
	}
	if err := c.Configure(func(s *grabber.Settings) {
		s.SeriesFolders, s.GuessLanguage, s.Language = true, true, "en"
	}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Мир Лидейл [ru]")

	s := runTwoChapters(t, c, func(*grabber.Settings) {})

	if s.Dir != want {
		t.Errorf("summary dir %q, want %q", s.Dir, want)
	}
}

// Guess Language alone does nothing, and keeps its value for later
func TestGuessLanguageNeedsSeriesFolders(t *testing.T) {
	c, _, root := newTestController(t, false, nil, chapters(1, 1)...)
	if err := c.Configure(func(s *grabber.Settings) { s.GuessLanguage = true }); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if d := c.Destination(); d != root {
		t.Errorf("Destination() = %q, want the root %q", d, root)
	}
	if !optionsOf(c.Settings()).GuessLanguage {
		t.Error("GuessLanguage lost its saved value")
	}
}

// a folder that can't be made stops the run before it starts
func TestSeriesFolderThatCantBeMadeRefusesStart(t *testing.T) {
	c, _, root := newTestController(t, false, nil, chapters(1, 1)...)
	// a file where the series folder would go
	if err := os.WriteFile(filepath.Join(root, "Test Series"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := c.Load("https://example.com/manga/test"); err != nil {
		t.Fatal(err)
	}
	if err := c.Configure(func(s *grabber.Settings) { s.SeriesFolders = true }); err != nil {
		t.Fatal(err)
	}
	if _, err := c.SelectRange("1"); err != nil {
		t.Fatal(err)
	}

	err := c.Start()

	if err == nil || !strings.Contains(err.Error(), "couldn't create the series folder") {
		t.Fatalf("Start = %v, want the series folder error", err)
	}
	if c.Running() {
		t.Error("the run started anyway")
	}
}

func TestOpenFolderDir(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "Old Series [en]")
	if err := os.Mkdir(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(root, "New Series")
	ran := filepath.Join(root, "Ran [ja]")

	tests := []struct {
		name        string
		destination string
		last        *Summary
		want        string
	}{
		{"nothing loaded", "", nil, root},
		{"series folders off", root, nil, root},
		{"series folder there already", existing, nil, existing},
		{"series folder not made yet", missing, nil, root},
		{"the last run wins", existing, &Summary{Dir: ran}, ran},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := openFolderDir(root, tt.destination, tt.last); got != tt.want {
				t.Errorf("openFolderDir = %q, want %q", got, tt.want)
			}
		})
	}
}

// both switches are off when gui.json predates them, and saved when set
func TestSeriesFolderOptionsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gui.json")
	if err := os.WriteFile(path, []byte(`{"outputDir": "/m", "format": "cbz"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := defaultSettings()
	if err := loadOptions(path, s); err != nil {
		t.Fatal(err)
	}
	if s.SeriesFolders || s.GuessLanguage {
		t.Fatalf("old file loaded as SeriesFolders %v, GuessLanguage %v; want both off", s.SeriesFolders, s.GuessLanguage)
	}

	o := optionsOf(*s)
	o.SeriesFolders, o.GuessLanguage = true, true
	if err := saveOptions(path, o, defaultPrefs()); err != nil {
		t.Fatal(err)
	}
	again := defaultSettings()
	if err := loadOptions(path, again); err != nil {
		t.Fatal(err)
	}
	if !again.SeriesFolders || !again.GuessLanguage || again.OutputDir != "/m" {
		t.Errorf("reloaded SeriesFolders %v, GuessLanguage %v, OutputDir %q", again.SeriesFolders, again.GuessLanguage, again.OutputDir)
	}
}
