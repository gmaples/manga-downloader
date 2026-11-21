// Copyright (C) 2023-2026 Òscar Casajuana Alonso

// Command manga-downloader-gui is the desktop front end of the CLI. It lives
// in its own module so the webview's cgo and its platform libraries never
// reach the CLI's module; see the gui skill.
package main

import (
	"embed"

	"github.com/elboletaire/manga-downloader/notice"
)

// Version and Tag are set at link time by the release workflow (-X), the same
// way the CLI gets its version: Version is the short commit sha, Tag the
// release tag. Both stay at their defaults in a plain build.
var (
	Version = "dev"
	Tag     = ""
)

// assets is the built frontend. It's produced by `pnpm build` in frontend/,
// which make build/gui and the release workflow run before compiling.
//
//go:embed all:frontend/dist
var assets embed.FS

// newSession returns the controller with the saved options applied, and the
// path they're saved at (empty when there is none). The controller is created
// first, so the warnings loading raises reach its log instead of stdout.
func newSession(notify func()) (*Controller, string) {
	settings := defaultSettings()
	ctrl := NewController(settings, notify)

	prefsPath, err := guiConfigPath()
	if err != nil {
		// the window still works, the options just won't outlive this run
		notice.Warn("%v", err)
		return ctrl, ""
	}
	if err := loadOptions(prefsPath, settings); err != nil {
		notice.Warn("ignoring the saved options: %v", err)
	}
	// nothing else holds the controller yet, so it's set without the lock
	ctrl.prefs = loadPrefs(prefsPath)
	ctrl.SetHistory(historyPath(prefsPath), loadHistory(historyPath(prefsPath)))

	return ctrl, prefsPath
}

// versionString is what the window's About text shows: the tag when there is
// one, the build's version otherwise
func versionString() string {
	if Tag != "" {
		return Tag
	}

	return Version
}
