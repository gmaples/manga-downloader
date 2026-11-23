// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package browser

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/elboletaire/manga-downloader/internal/filelock"
)

// profileSlots is how many saved profiles can be in use at once (the app and
// a few command lines running together); a run that finds them all taken
// starts with a throwaway profile, as every run did before profiles were kept
const profileSlots = 4

// profileRoot is the folder the saved browser profiles live in; a var so
// tests can point it at a temporary folder, or "" to keep none
var profileRoot = func() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "manga-downloader", "browser")
}

// profile is a saved browser profile this process holds. Keeping the
// profile between runs keeps the cookies a site's check sets once passed
// (joilmang.com's Vercel check: about an hour), so later runs within that
// time skip the check, and a window that had to show stays away.
type profile struct {
	dir     string
	release func()
}

// claimProfile takes the first saved profile no other run is using, or
// returns nil when there's none (no cache folder, all slots taken), and the
// browser then starts with a throwaway profile
func claimProfile() *profile {
	root := profileRoot()
	if root == "" {
		return nil
	}
	for n := 1; n <= profileSlots; n++ {
		dir := filepath.Join(root, "profile")
		if n > 1 {
			dir = fmt.Sprintf("%s-%d", dir, n)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil
		}
		// the lock file sits beside the profile, as Chrome owns its insides
		release, err := filelock.TryLock(dir + ".lock")
		if err != nil {
			continue
		}
		return &profile{dir: dir, release: release}
	}
	return nil
}
