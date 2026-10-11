// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"time"

	"github.com/yousysadmin/gosint/internal/models/attachment"
)

// Snapshot is the state of some nodes and edges at one moment, with
// the records of the files attached to the nodes.
type Snapshot struct {
	Nodes []Node                  `json:"nodes"`
	Edges []Edge                  `json:"edges"`
	Files []attachment.Attachment `json:"files,omitempty"`
}

// Change is one hand edit of a sketch's graph: what the touched nodes
// and edges were before it and after it, so it can be undone by
// writing Before back, and redone by writing After.
type Change struct {
	ID       string `json:"id"`
	SketchID string `json:"sketch_id"`
	UserID   string `json:"user_id"`

	// Kind says what the edit was, as the Undo button reads it.
	Kind      string    `json:"kind"`
	Before    Snapshot  `json:"before"`
	After     Snapshot  `json:"after"`
	Undone    bool      `json:"undone"`
	CreatedAt time.Time `json:"created_at"`
}

// EdgeKey is an edge's identity within its sketch.
func EdgeKey(e Edge) string { return e.SourceID + "\x00" + e.TargetID + "\x00" + e.Label }
