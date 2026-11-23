// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package cmd

import (
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/elevated"
)

// --force goes only with --delete-series, and only elevated
func TestCheckForce(t *testing.T) {
	defer func() { deleteSeries, forceDelete = false, false }()

	deleteSeries, forceDelete = false, false
	if err := checkForce(); err != nil {
		t.Errorf("no flags: %v", err)
	}
	deleteSeries, forceDelete = false, true
	if err := checkForce(); err == nil || !strings.Contains(err.Error(), "--delete-series") {
		t.Errorf("--force alone: %v", err)
	}
	deleteSeries, forceDelete = true, true
	err := checkForce()
	if elevated.Is() != (err == nil) {
		t.Errorf("elevated %v, err %v", elevated.Is(), err)
	}
}
