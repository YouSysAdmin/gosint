// Gosint, Copyright (c) 2026 YouSysAdmin

// Package vaultkey is a stored, encrypted API key.
package vaultkey

import "time"

// Key is one secret an account keeps for transforms.
type Key struct {
	ID      string `json:"id"`
	OwnerID string `json:"owner_id"`
	Name    string `json:"name"`
	// Stored is the secret sealed by the vault under the owner's id.
	Stored    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
