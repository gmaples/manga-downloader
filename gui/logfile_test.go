// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// every log line goes to the file too, after a start line, masked as the pane shows it
func TestLogFileGetsEveryLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), logFileName)
	c := NewController(defaultSettings(), func() {})
	c.SetLogFile(path)
	c.mu.Lock()
	c.appendLineLocked(LogLine{Level: LogError, Text: "chapter 3: page 2: received 400"})
	c.appendLineLocked(LogLine{Level: LogInfo, Text: "two\nlines"})
	c.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	stamp := `^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d `
	for i, want := range []string{stamp + `info: app started, `, stamp + `error: chapter 3: page 2: received 400$`, stamp + `info: two$`, `^    lines$`} {
		if i >= len(got) || !regexp.MustCompile(want).MatchString(got[i]) {
			t.Fatalf("log file is %q, line %d should match %s", data, i+1, want)
		}
	}
}

// a file past logFileMax is moved to log-old.txt at the start, and a new one begun
func TestLogFileRollsOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, logFileName)
	if err := os.WriteFile(path, []byte(strings.Repeat("x", logFileMax+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	c := NewController(defaultSettings(), func() {})
	c.SetLogFile(path)
	if fi, err := os.Stat(filepath.Join(dir, logOldName)); err != nil || fi.Size() != logFileMax+1 {
		t.Fatalf("log-old.txt: %v %v, want the old file", fi, err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "app started") || len(data) > 200 {
		t.Errorf("log.txt is %q, want a new file", data)
	}
}

// a log file that can't be opened is said in the log, which carries on
func TestLogFileThatCantOpen(t *testing.T) {
	c := NewController(defaultSettings(), func() {})
	c.SetLogFile(filepath.Join(t.TempDir(), "missing", "dir", logFileName))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.logFile != nil || len(c.log) != 1 || !strings.Contains(c.log[0].Text, "isn't saved to a file") {
		t.Errorf("log %+v, file %v", c.log, c.logFile)
	}
}
