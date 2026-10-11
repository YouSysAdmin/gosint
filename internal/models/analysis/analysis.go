// Gosint, Copyright (c) 2026 YouSysAdmin

// Package analysis is a written analysis of an investigation.
package analysis

import (
	"encoding/json/jsontext"
	"time"
)

// Analysis is one document. Content is the editor's document as JSON,
// kept as it came, the server never reads inside it.
type Analysis struct {
	ID              string         `json:"id"`
	InvestigationID string         `json:"investigation_id"`
	OwnerID         string         `json:"owner_id"`
	Title           string         `json:"title"`
	Content         jsontext.Value `json:"content"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}
