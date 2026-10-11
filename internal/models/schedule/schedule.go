// Gosint, Copyright (c) 2026 YouSysAdmin

// Package schedule is a flow run again on a sketch every so often.
package schedule

import "time"

// Schedule runs a flow on a sketch every Every, on NodeIDs or, when
// empty, on every entity of the flow's input type at the time. The
// scans it queued last let the console say what a run found.
type Schedule struct {
	ID        string     `json:"id"`
	SketchID  string     `json:"sketch_id"`
	OwnerID   string     `json:"owner_id"`
	FlowID    string     `json:"flow_id"`
	FlowName  string     `json:"flow_name"`
	NodeIDs   []string   `json:"node_ids"`
	Every     int        `json:"every_seconds"`
	Enabled   bool       `json:"enabled"`
	Version   int        `json:"-"`
	LastRunAt *time.Time `json:"last_run_at"`

	// LastScanID is the newest run, PrevScanID the one before, so a
	// diff between the two can be read off the graph's provenance.
	LastScanID string    `json:"last_scan_id"`
	PrevScanID string    `json:"prev_scan_id"`
	LastError  string    `json:"last_error"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Due says whether the schedule should run at now.
func (s Schedule) Due(now time.Time) bool {
	if !s.Enabled {
		return false
	}

	return s.LastRunAt == nil || !now.Before(s.LastRunAt.Add(time.Duration(s.Every)*time.Second))
}
