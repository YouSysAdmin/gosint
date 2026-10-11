// Gosint, Copyright (c) 2026 YouSysAdmin

// Package investigation is the investigation record and its access
// model.
package investigation

import "time"

// Investigation is one case, holding sketches and the people working it.
type Investigation struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	OwnerID     string    `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Member is one person's role on an investigation.
type Member struct {
	InvestigationID string    `json:"investigation_id"`
	UserID          string    `json:"user_id"`
	Role            Role      `json:"role"`
	CreatedAt       time.Time `json:"created_at"`
}

// WithRole is an investigation as one member sees it.
type WithRole struct {
	Investigation
	Role Role `json:"current_user_role"`
}
