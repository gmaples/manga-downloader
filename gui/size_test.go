package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/elboletaire/manga-downloader/core"
)

// A size travels as core.Bytes: log.txt's English text reads as before, and
// the window gets the bare number to write the language's way
func TestSizeArgIsTextInTheLogAndANumberOnTheWire(t *testing.T) {
	m := msg("log.removed", map[string]any{"count": 3, "size": core.Bytes(6_400_000_000), "folder": "/m/X"})
	if !strings.Contains(m.Text, "6.4 GB") {
		t.Errorf("Text = %q, want it to contain 6.4 GB", m.Text)
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"size":6400000000`) {
		t.Errorf("json = %s, want the size as a number", data)
	}
}
