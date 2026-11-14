// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package packer

import (
	"archive/zip"
	"bytes"
	"io"
	"path/filepath"
	"testing"

	"github.com/elboletaire/manga-downloader/downloader"
	"github.com/elboletaire/manga-downloader/grabber"
)

// entryNames opens the zip at path and returns its entry names, in order.
func entryNames(t *testing.T, path string) []string {
	t.Helper()

	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("opening archive: %v", err)
	}
	defer r.Close()

	names := make([]string, len(r.File))
	for i, f := range r.File {
		names[i] = f.Name
	}
	return names
}

func TestArchiveCBZNoFiles(t *testing.T) {
	err := ArchiveCBZ(filepath.Join(t.TempDir(), "empty.cbz"), []File{}, func(page, progress int) {})
	if err == nil {
		t.Fatal("expected an error when archiving zero files, got nil")
	}
}

func TestArchiveCBZEntryNames(t *testing.T) {
	// Simulate a bundle: two chapters, each with its own folder and page
	// numbering restarting at 000, per issue #47.
	files := []File{
		{Name: "Chapter 0001/000.jpg", Data: []byte("ch1-p0")},
		{Name: "Chapter 0001/001.jpg", Data: []byte("ch1-p1")},
		{Name: "Chapter 0002/000.jpg", Data: []byte("ch2-p0")},
	}

	path := filepath.Join(t.TempDir(), "bundle.cbz")
	if err := ArchiveCBZ(path, files, func(page, progress int) {}); err != nil {
		t.Fatalf("ArchiveCBZ: %v", err)
	}

	names := entryNames(t, path)
	want := []string{"Chapter 0001/000.jpg", "Chapter 0001/001.jpg", "Chapter 0002/000.jpg"}
	if len(names) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("entry %d = %q, want %q", i, n, want[i])
		}
	}

	// Both chapters must independently contain a 000.jpg (no global renumbering).
	seen000 := 0
	for _, n := range names {
		if filepath.Base(n) == "000.jpg" {
			seen000++
		}
	}
	if seen000 != 2 {
		t.Errorf("expected 2 entries named 000.jpg (one per chapter folder), got %d", seen000)
	}
}

func TestPaddedChapterNumber(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1, "0001"},
		{10, "0010"},
		{10.5, "0010.5"},
		{186, "0186"},
		{1186, "1186"},
		{1186.5, "1186.5"},
		{0, "0000"},
	}

	for _, c := range cases {
		if got := paddedChapterNumber(c.in); got != c.want {
			t.Errorf("paddedChapterNumber(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeSite is a minimal grabber.Site implementation for testing PackBundle's
// entry-naming behavior without hitting a real site.
type fakeSite struct {
	title     string
	template  string
	convert   grabber.ConvertFormats
	format    string
	direction string
}

func (f *fakeSite) Test() (bool, error)                           { return true, nil }
func (f *fakeSite) FetchChapters() (grabber.Filterables, []error) { return nil, nil }
func (f *fakeSite) FetchChapter(grabber.Filterable) (*grabber.Chapter, error) {
	return nil, nil
}
func (f *fakeSite) FetchTitle() (string, error) { return f.title, nil }
func (f *fakeSite) BaseUrl() string             { return "https://example.com" }
func (f *fakeSite) GetFilenameTemplate() string { return f.template }
func (f *fakeSite) GetFormat() string {
	if f.format == "" {
		return FormatCBZ
	}
	return f.format
}
func (f *fakeSite) GetReadingDirection() string { return f.direction }
func (f *fakeSite) GetConvertImages() grabber.ConvertFormats {
	return f.convert
}
func (f *fakeSite) GetJPEGQuality() int { return grabber.JPEGQualityDefault }
func (f *fakeSite) GetCompress() bool   { return false }
func (f *fakeSite) GetMaxConcurrency() grabber.MaxConcurrency {
	return grabber.MaxConcurrency{Chapters: 1, Pages: 1}
}
func (f *fakeSite) GetPreferredLanguage() string  { return "" }
func (f *fakeSite) GetPreferredScanlator() string { return "" }
func (f *fakeSite) GetRetries() uint8             { return 0 }

// TestPackBundleConvertsPages is the only test covering the whole path the
// --convert-images setting travels: from the Site interface, through the
// packer, into the archive entry names.
func TestPackBundleConvertsPages(t *testing.T) {
	site := &fakeSite{
		title:    "Test Series",
		template: FilenameTemplateDefault,
		convert:  grabber.ConvertFormats{grabber.ConvertAVIF: true},
	}

	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1},
			Files:   []*downloader.File{{Data: avifBytes(t, gradientImage(16, 16))}},
		},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "1", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}

	names := entryNames(t, filepath.Join(dir, filename))
	if len(names) != 1 || names[0] != "Chapter 0001/000.jpg" {
		t.Errorf("got %v, want [Chapter 0001/000.jpg]", names)
	}
}

func TestPackBundleEntryNames(t *testing.T) {
	site := &fakeSite{title: "Test Series", template: FilenameTemplateDefault}

	chapters := []*DownloadedChapter{
		{
			Chapter: &grabber.Chapter{Number: 1},
			Files: []*downloader.File{
				{Data: []byte("ch1-p0")},
				{Data: []byte("ch1-p1")},
			},
		},
		{
			Chapter: &grabber.Chapter{Number: 10.5},
			Files: []*downloader.File{
				{Data: []byte("ch10.5-p0")},
			},
		},
	}

	dir := t.TempDir()
	filename, err := PackBundle(dir, site, chapters, "1-10.5", func(page, progress int) {})
	if err != nil {
		t.Fatalf("PackBundle: %v", err)
	}

	names := entryNames(t, filepath.Join(dir, filename))
	want := []string{
		"Chapter 0001/000.jpg",
		"Chapter 0001/001.jpg",
		"Chapter 0010.5/000.jpg",
	}
	if len(names) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(names), len(want), names)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("entry %d = %q, want %q", i, n, want[i])
		}
	}
}

// TestArchiveCBZStoresPages checks pages are stored as they are, with their
// CRC and sizes in the local header (no data descriptor, flag bit 3), and a
// non-ASCII name flagged as UTF-8, and that they read back unchanged
func TestArchiveCBZStoresPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stored.cbz")
	files := []File{
		{Name: "000.jpg", Data: []byte{0xff, 0xd8, 0xff, 1, 2, 3}},
		{Name: "Capítulo 0001/001.png", Data: []byte("\x89PNG\r\n\x1a\nrest")},
	}
	if err := ArchiveCBZ(path, files, func(int, int) {}); err != nil {
		t.Fatal(err)
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if len(r.File) != len(files) {
		t.Fatalf("got %d entries, want %d", len(r.File), len(files))
	}
	for i, f := range r.File {
		if f.Name != files[i].Name {
			t.Errorf("entry %d is %q, want %q", i, f.Name, files[i].Name)
		}
		if f.Method != zip.Store {
			t.Errorf("%s: method %d, want Store", f.Name, f.Method)
		}
		if f.Flags&0x8 != 0 {
			t.Errorf("%s: has a data descriptor", f.Name)
		}
		if utf8 := f.Flags&0x800 != 0; utf8 != (i == 1) {
			t.Errorf("%s: UTF-8 flag %v", f.Name, utf8)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(rc) // checks the CRC at EOF
		rc.Close()
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		if !bytes.Equal(got, files[i].Data) {
			t.Errorf("%s: data changed", f.Name)
		}
	}
}
