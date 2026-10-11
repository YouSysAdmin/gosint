// Gosint, Copyright (c) 2026 YouSysAdmin

// Package graph is the stored shape of a sketch's entities and the
// relationships between them.
package graph

import "time"

// Node is one entity on a sketch. (SketchID, Type, Key) is its
// identity, so finding the same entity again updates this node. Key is
// the primary field's value, Label only how the node is shown.
type Node struct {
	ID         string         `json:"id"`
	SketchID   string         `json:"sketch_id"`
	Type       string         `json:"type"`
	Key        string         `json:"key"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Metadata   map[string]any `json:"metadata"`
	Style
	X         *float64  `json:"x"`
	Y         *float64  `json:"y"`
	Version   int       `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Style is how a node is drawn. An empty field means the type default.
type Style struct {
	Color string `json:"color"`
	Icon  string `json:"icon"`
	Image string `json:"image"`
	Flag  string `json:"flag"`
	Shape string `json:"shape"`
	Size  *int   `json:"size"`
}

// Edge is one directed relationship between two nodes of a sketch.
// (SketchID, SourceID, TargetID, Label) is its identity.
type Edge struct {
	ID         string         `json:"id"`
	SketchID   string         `json:"sketch_id"`
	SourceID   string         `json:"source"`
	TargetID   string         `json:"target"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// NodePatch is a partial node update. A nil field is left as it is,
// and Properties and Metadata are merged key by key.
type NodePatch struct {
	Key        *string
	Label      *string
	Properties map[string]any
	Metadata   map[string]any
	Color      *string
	Icon       *string
	Image      *string
	Flag       *string
	Shape      *string
	Size       *int
}

// EdgePatch is a partial edge update. Properties are merged key by key.
type EdgePatch struct {
	Label      *string
	Properties map[string]any
}

// EdgeRef is an edge of an import, its ends as indexes into the nodes
// of the same import.
type EdgeRef struct {
	Source     int
	Target     int
	Label      string
	Properties map[string]any
}

// Position is where the console last drew a node.
type Position struct {
	NodeID string
	X, Y   float64
}

// TypePurpose is the keyed hash an entity type is stored and matched
// by, in nodes.node_type and wherever a type is filtered on.
const TypePurpose = "gosint/node-type"
