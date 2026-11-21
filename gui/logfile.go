// Copyright (C) 2023-2026 Òscar Casajuana Alonso

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The log pane keeps its lines in memory only, so they're also appended to a
// plain text file next to gui.json, one "time level: text" line each, to be
// read after the app is closed. When the file has grown past logFileMax at a
// start, it's renamed to logOldName first, so the two files never hold much
// more than twice that.
const (
	logFileName = "log.txt"
	logOldName  = "log-old.txt"
	logFileMax  = 5 << 20
)

// logFilePath is log.txt, next to gui.json
func logFilePath(prefsPath string) string {
	return filepath.Join(filepath.Dir(prefsPath), logFileName)
}

// openLogFile opens path for appending, moving a file past logFileMax to
// log-old.txt first
func openLogFile(path string) (*os.File, error) {
	if fi, err := os.Stat(path); err == nil && fi.Size() > logFileMax {
		if err := os.Rename(path, filepath.Join(filepath.Dir(path), logOldName)); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
}

// SetLogFile has every log line from now on appended to the file at path too.
// A file that can't be opened is reported in the log, and the log carries on
// without it.
func (c *Controller) SetLogFile(path string) {
	f, err := openLogFile(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.appendLineLocked(LogLine{Level: LogWarn, Text: fmt.Sprintf("the log isn't saved to a file: %v", err)})
		return
	}
	c.logFile = f
	c.writeLogFileLocked(LogLine{Level: LogInfo, Text: "app started, " + versionString()})
}

// logLevelNames are the levels as the file writes them
var logLevelNames = map[LogLevel]string{LogInfo: "info", LogWarn: "warning", LogError: "error"}

// writeLogFileLocked appends line to the log file, if there is one. A failed
// write drops the file, so a full disk doesn't fail every line. c.mu is held.
func (c *Controller) writeLogFileLocked(line LogLine) {
	if c.logFile == nil {
		return
	}
	text := strings.ReplaceAll(strings.TrimRight(line.Text, "\r\n"), "\n", "\n    ")
	entry := fmt.Sprintf("%s %s: %s\n", time.Now().Format("2006-01-02 15:04:05"), logLevelNames[line.Level], text)
	if _, err := c.logFile.WriteString(entry); err != nil {
		c.logFile.Close()
		c.logFile = nil
	}
}
