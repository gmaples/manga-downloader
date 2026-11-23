// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package trash

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// noBin makes every drive one with no Recycle Bin for the test
func noBin(t *testing.T) {
	t.Helper()
	was := HasRecycleBin
	HasRecycleBin = func(string) bool { return false }
	t.Cleanup(func() { HasRecycleBin = was })
}

// on a drive with no Recycle Bin, Files refuses the file and leaves it where
// it is, rather than deleting it for good; a missing one still counts as moved
func TestFilesRefusesWithoutRecycleBin(t *testing.T) {
	noBin(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "gone.cbz")

	failed := Files([]string{p, gone})
	if !errors.Is(failed[p], ErrNoRecycleBin) {
		t.Errorf("failed[%s] = %v, want ErrNoRecycleBin", p, failed[p])
	}
	if _, ok := failed[gone]; ok {
		t.Errorf("a missing file failed: %v", failed[gone])
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("the file isn't there any more: %v", err)
	}
}

// Remove deletes for good, and a missing file counts as removed
func TestRemove(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.cbz")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	failed := Remove([]string{p, filepath.Join(dir, "gone.cbz")})
	if len(failed) != 0 {
		t.Fatalf("failed: %v", failed)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("still there: %v", err)
	}
}
