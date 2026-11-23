package trash

import (
	"os"
	"path/filepath"
	"testing"
)

// The temp folder is on C:, which has a Recycle Bin: Files moves a file
// there, and nothing fails
func TestFilesOnWindowsGoToTheRecycleBin(t *testing.T) {
	dir := t.TempDir()
	if !HasRecycleBin(dir) {
		t.Fatalf("HasRecycleBin(%s) = false, want true on C:", dir)
	}
	p := filepath.Join(dir, "a.cbz")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if failed := Files([]string{p}); len(failed) != 0 {
		t.Fatalf("Files failed: %v", failed)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("the file is still there: %v", err)
	}
}
