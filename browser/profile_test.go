// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"path/filepath"
	"testing"
)

func withProfileRoot(t *testing.T, root string) {
	t.Helper()
	prev := profileRoot
	profileRoot = func() string { return root }
	t.Cleanup(func() { profileRoot = prev })
}

// each run takes the first profile no other run holds, and gets none once
// every slot is taken; a released one is taken again
func TestClaimProfileSlots(t *testing.T) {
	root := t.TempDir()
	withProfileRoot(t, root)

	var held []*profile
	for n := 1; n <= profileSlots; n++ {
		p := claimProfile()
		if p == nil {
			t.Fatalf("claim %d: got none", n)
		}
		want := filepath.Join(root, "profile")
		if n > 1 {
			want += "-" + string(rune('0'+n))
		}
		if p.dir != want {
			t.Errorf("claim %d: dir %q, want %q", n, p.dir, want)
		}
		held = append(held, p)
	}
	if p := claimProfile(); p != nil {
		t.Errorf("claim with every slot taken: got %q, want none", p.dir)
	}

	held[1].release()
	p := claimProfile()
	if p == nil || p.dir != held[1].dir {
		t.Fatalf("claim after a release: got %+v, want %q", p, held[1].dir)
	}
	p.release()
	for i, h := range held {
		if i != 1 {
			h.release()
		}
	}
}

func TestClaimProfileWithoutACacheFolder(t *testing.T) {
	withProfileRoot(t, "")
	if p := claimProfile(); p != nil {
		t.Errorf("got %q, want none (a throwaway profile)", p.dir)
	}
}
