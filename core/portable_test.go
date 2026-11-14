// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package core

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/grabber"
	"github.com/elboletaire/manga-downloader/packer"
)

// hostileTitles are series and chapter titles that break naive names on some
// filesystem: Windows/FAT characters, control characters, trailing dots and
// spaces, dot names Unix hides, device names, traversal, and names too long
// for ext4/f2fs (255 bytes) once UTF-8 encoded
var hostileTitles = []string{
	`Re:Zero? "Kara" <Hajimeru> | Isekai*`,
	"tab\there\nnewline\x00nul\x7fdel",
	"Ends with dots and spaces. . .",
	".hack//Sign",
	"..",
	"CON",
	"lpt1.txt",
	`..\..\escape/../again`,
	strings.Repeat("長い題名", 30),
	strings.Repeat("Ω", 200),
	"Ván-Pis 第1話 원피스 Ünïcödé",
}

// TestPortableNamesOnDisk creates every name the series folders and the
// archives would get for hostileTitles, then reads the folder back and checks
// each name is there exactly as computed: what a reader and skip-existing
// later look for. It runs in a temp dir, or in $MD_PORTABLE_DIR to check
// another filesystem (an SD card, a FAT or exFAT stick, a network share);
// MD_PORTABLE_KEEP=1 leaves the names there to copy elsewhere.
func TestPortableNamesOnDisk(t *testing.T) {
	root := os.Getenv("MD_PORTABLE_DIR")
	if root == "" {
		root = t.TempDir()
	} else {
		var err error
		if root, err = os.MkdirTemp(root, "md-portable-"); err != nil {
			t.Fatal(err)
		}
		if os.Getenv("MD_PORTABLE_KEEP") == "" {
			t.Cleanup(func() { os.RemoveAll(root) })
		}
	}

	for i, title := range hostileTitles {
		for _, lang := range []string{"", "zh-Hant"} {
			dir := SeriesDir(root, title, lang)
			name := filepath.Base(dir)
			if filepath.Dir(dir) != root {
				t.Errorf("title %d: %q is not one folder inside the root", i, dir)
				continue
			}
			if len(name) > packer.MaxNameBytes {
				t.Errorf("title %d: folder name is %d bytes", i, len(name))
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Errorf("title %d: making folder %q: %v", i, name, err)
				continue
			}
			var want []string
			for _, format := range []string{packer.FormatCBZ, packer.FormatCBR, packer.FormatEPUB, packer.FormatRaw} {
				chapter := &grabber.Chapter{Title: title, Number: 1, Label: grabber.Label{Style: grabber.LabelNumbered}}
				chapter.SetGroups(title)
				file, err := packer.ChapterName(title, packer.FilenameTemplateDefault, format, chapter)
				if err != nil {
					t.Fatal(err)
				}
				if len(file) > packer.MaxNameBytes {
					t.Errorf("title %d %s: file name is %d bytes", i, format, len(file))
				}
				path := filepath.Join(dir, file)
				if format == packer.FormatRaw {
					err = os.Mkdir(path, 0o755)
				} else {
					err = os.WriteFile(path, []byte("x"), 0o644)
				}
				if err != nil {
					t.Errorf("title %d %s: creating %q: %v", i, format, file, err)
					continue
				}
				want = append(want, file)
				if !packer.Exists(dir, file) {
					t.Errorf("title %d %s: Exists can't find %q", i, format, file)
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range entries {
				got = append(got, e.Name())
			}
			slices.Sort(got)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("title %d: folder %q holds\n %q\nwant\n %q", i, name, got, want)
			}
		}
	}
}
