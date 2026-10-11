// Gosint, Copyright (c) 2026 YouSysAdmin

// Package flow is a stored chain of transforms and its definition.
package flow

import "time"

// Root is the parent name of a step fed the flow's input nodes.
const Root = "input"

// Flow is one account's chain of transforms.
type Flow struct {
	ID          string     `json:"id"`
	OwnerID     string     `json:"owner_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Definition  Definition `json:"definition"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Definition is a tree of steps from an input entity type.
type Definition struct {
	// Input is the entity type the flow starts from.
	Input string `json:"input"`
	Steps []Step `json:"steps"`

	// Layout is the editor's own state, kept and never read by the server.
	Layout map[string]any `json:"layout"`
}

// Step runs one transform on the nodes its parent wrote. After is the
// parent step's id, or Root.
type Step struct {
	ID        string            `json:"id"`
	Transform string            `json:"transform"`
	After     string            `json:"after"`
	Params    map[string]string `json:"params"`
}
