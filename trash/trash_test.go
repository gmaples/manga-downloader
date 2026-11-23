// Copyright (C) 2023-2026 Òscar Casajuana Alonso

//go:build !windows && !darwin

package trash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// a file goes to the home trash with the .trashinfo desktops restore it from,
// a second of the same name gets its own, and a missing one counts as moved
func TestFilesFreedesktop(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	dir := t.TempDir()
	a := filepath.Join(dir, "a b.cbz")
	sub := filepath.Join(dir, "sub")
	b := filepath.Join(sub, "a b.cbz")
	for _, p := range []string{a, b} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(p), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	failed := Files([]string{a, b, filepath.Join(dir, "gone.cbz")})
	if len(failed) != 0 {
		t.Fatalf("failed: %v", failed)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still there", p)
		}
	}
	files := filepath.Join(data, "Trash", "files")
	for name, from := range map[string]string{"a b.cbz": a, "a b (2).cbz": b} {
		got, err := os.ReadFile(filepath.Join(files, name))
		if err != nil || string(got) != from {
			t.Errorf("%s: %q, %v", name, got, err)
		}
		info, err := os.ReadFile(filepath.Join(data, "Trash", "info", name+".trashinfo"))
		if err != nil || !strings.Contains(string(info), "Path="+strings.ReplaceAll(from, " ", "%20")) {
			t.Errorf("%s info: %q, %v", name, info, err)
		}
	}
}
