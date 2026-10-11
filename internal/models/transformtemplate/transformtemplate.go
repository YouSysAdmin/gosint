// Gosint, Copyright (c) 2026 YouSysAdmin

// Package transformtemplate is a stored YAML transform.
package transformtemplate

import "time"

// Template is one transform an account declared as YAML. Source is the
// YAML, the rest is read from it on save.
type Template struct {
	ID          string    `json:"id"`
	OwnerID     string    `json:"owner_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	InputType   string    `json:"input_type"`
	OutputType  string    `json:"output_type"`
	Source      string    `json:"source"`
	Public      bool      `json:"is_public"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
