// Gosint, Copyright (c) 2026 YouSysAdmin

// Package logentry is one line of a sketch's activity log.
package logentry

import "time"

// Level is a log line's kind.
type Level string

// The levels.
const (
	LevelDebug       Level = "debug"
	LevelInfo        Level = "info"
	LevelSuccess     Level = "success"
	LevelWarning     Level = "warning"
	LevelFailed      Level = "failed"
	LevelPending     Level = "pending"
	LevelRunning     Level = "running"
	LevelCompleted   Level = "completed"
	LevelGraphAppend Level = "graph_append"
)

// Entry is one log line.
type Entry struct {
	ID        string         `json:"id"`
	SketchID  string         `json:"sketch_id"`
	ScanID    string         `json:"scan_id"`
	Level     Level          `json:"level"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data"`
	CreatedAt time.Time      `json:"created_at"`
}
