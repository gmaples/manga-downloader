package filelock

import (
	"path/filepath"
	"testing"
)

// A held lock refuses a second holder, even in the same program, until it
// is released
func TestTryLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	release, err := TryLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := TryLock(path); err == nil {
		again()
		t.Fatal("a second TryLock got the lock while it was held")
	}
	release()
	again, err := TryLock(path)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	again()
}
