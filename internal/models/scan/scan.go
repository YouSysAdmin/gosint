// Gosint, Copyright (c) 2026 YouSysAdmin

// Package scan is one run of a transform or a flow, and its place in
// the queue.
package scan

import "time"

// Status is where a scan is in its life.
type Status string

// The statuses. Pending and running are live, the rest are final.
const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Final reports whether s will not change again.
func (s Status) Final() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCancelled
}

// Scan is one run.
type Scan struct {
	ID       string `json:"id"`
	SketchID string `json:"sketch_id"`
	OwnerID  string `json:"owner_id"`

	// Kind picks the handler, Name what it runs, such as
	// transform / domain_to_ip.
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Status Status         `json:"status"`
	Input  map[string]any `json:"input"`

	// Details is what the run produced, Error why it failed.
	Details     map[string]any `json:"details"`
	Error       string         `json:"error"`
	Attempts    int            `json:"attempts"`
	ClaimedBy   string         `json:"-"`
	LeaseUntil  *time.Time     `json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at"`
	CompletedAt *time.Time     `json:"completed_at"`
}
