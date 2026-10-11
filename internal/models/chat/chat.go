// Gosint, Copyright (c) 2026 YouSysAdmin

// Package chat is a conversation with the assistant about a sketch.
package chat

import "time"

// Chat is one conversation, an account's own, about one sketch.
type Chat struct {
	ID       string `json:"id"`
	SketchID string `json:"sketch_id"`
	OwnerID  string `json:"owner_id"`
	Title    string `json:"title"`

	// Entities are the ids every question of the chat was asked about,
	// the union of the turns' Context, so a chat is found again by what
	// it was about. Filled by List, not stored.
	Entities  []string  `json:"entities"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is one turn. Context is the ids of the entities selected when
// the question was asked.
type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Context   []string  `json:"context"`
	CreatedAt time.Time `json:"created_at"`
}
