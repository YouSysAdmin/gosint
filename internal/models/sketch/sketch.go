// Gosint, Copyright (c) 2026 YouSysAdmin

// Package sketch is the sketch record, one graph canvas inside an
// investigation.
package sketch

import "time"

// Sketch is one graph canvas.
type Sketch struct {
	ID              string    `json:"id"`
	InvestigationID string    `json:"investigation_id"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Status          string    `json:"status"`
	OwnerID         string    `json:"owner_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
