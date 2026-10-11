// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"encoding/json/jsontext"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
)

// styleInput is how a node is drawn. Empty means the type default.
type styleInput struct {
	Color string `json:"color" validate:"max=32"   normalize:"trim"`
	Icon  string `json:"icon"  validate:"max=128"  normalize:"trim"`
	Image string `json:"image" validate:"max=4096" normalize:"trim"`
	Flag  string `json:"flag"  validate:"max=32"   normalize:"trim"`
	Shape string `json:"shape" validate:"max=32"   normalize:"trim"`
	Size  *int   `json:"size"  validate:"omitnil,min=1,max=1000"`
}

// nodeInput is a node to add. An existing node of the same type and
// label is merged into instead. The label comes from the type's main
// field, and label is used only when the type computes none.
type nodeInput struct {
	Type       string         `json:"type"  validate:"required,nodetype"`
	Label      string         `json:"label" validate:"max=1024" normalize:"trim"`
	Properties map[string]any `json:"properties"`
	Metadata   map[string]any `json:"metadata"`
	styleInput
	X *float64 `json:"x"`
	Y *float64 `json:"y"`
}

// patchNodeInput changes a node. Properties and metadata merge key by key.
type patchNodeInput struct {
	Label      *string        `json:"label" validate:"omitnil,notblank,max=1024" normalize:"trim"`
	Properties map[string]any `json:"properties"`
	Metadata   map[string]any `json:"metadata"`
	Color      *string        `json:"color" validate:"omitnil,max=32"`
	Icon       *string        `json:"icon"  validate:"omitnil,max=128"`
	Image      *string        `json:"image" validate:"omitnil,max=4096"`
	Flag       *string        `json:"flag"  validate:"omitnil,max=32"`
	Shape      *string        `json:"shape" validate:"omitnil,max=32"`
	Size       *int           `json:"size"  validate:"omitnil,min=1,max=1000"`
}

// positionInput is where the console drew one node.
type positionInput struct {
	NodeID string  `json:"node_id" validate:"required,uuid"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// positionsInput moves many nodes at once, after a layout or a drag.
type positionsInput struct {
	Positions []positionInput `json:"positions" validate:"required,max=10000,dive"`
}

// deleteNodesInput names nodes to delete, and whether what was found
// through them goes too.
type deleteNodesInput struct {
	IDs            []string `json:"ids" validate:"required,min=1,max=10000,dive,uuid"`
	WithDependents bool     `json:"with_dependents"`
}

// idsInput names nodes or edges.
type idsInput struct {
	IDs []string `json:"ids" validate:"required,min=1,max=10000,dive,uuid"`
}

// mergeNodeInput is what the merged node becomes. Type and label are
// required unless keep_id names one of the merged nodes.
type mergeNodeInput struct {
	Type       string         `json:"type"  validate:"omitempty,nodetype"`
	Label      string         `json:"label" validate:"max=1024" normalize:"trim"`
	Properties map[string]any `json:"properties"`
}

// mergeInput folds several nodes into one.
type mergeInput struct {
	NodeIDs []string       `json:"node_ids" validate:"required,min=2,max=1000,dive,uuid"`
	KeepID  string         `json:"keep_id"  validate:"omitempty,uuid"`
	Node    mergeNodeInput `json:"node"`
}

// edgeInput is a relationship to add.
type edgeInput struct {
	Source     string         `json:"source" validate:"required,uuid"`
	Target     string         `json:"target" validate:"required,uuid"`
	Label      string         `json:"label"  validate:"required,notblank,max=128" normalize:"trim"`
	Properties map[string]any `json:"properties"`
}

// patchEdgeInput changes a relationship. Properties merge key by key.
type patchEdgeInput struct {
	Label      *string        `json:"label" validate:"omitnil,notblank,max=128" normalize:"trim"`
	Properties map[string]any `json:"properties"`
}

// graphResponse answers a whole sketch graph or a neighborhood.
type graphResponse struct {
	Nodes []graphmodel.Node `json:"nodes"`
	Edges []graphmodel.Edge `json:"edges"`
}

// nodeResponse answers a single node.
type nodeResponse struct {
	Node graphmodel.Node `json:"node"`
}

// edgeResponse answers a single edge.
type edgeResponse struct {
	Edge graphmodel.Edge `json:"edge"`
}

// countResponse answers a bulk change.
type countResponse struct {
	Count int `json:"count"`
}

// idsResponse lists node ids.
type idsResponse struct {
	IDs []string `json:"ids"`
}

// exportInput asks for an export, sealed when a passphrase is given.
type exportInput struct {
	Passphrase string `json:"passphrase" validate:"max=500"`
}

// exportFile is one attached file inside an export, its bytes base64.
// The id lets an import point a node's picture at the file again.
type exportFile struct {
	ID          string `json:"id"`
	Node        string `json:"node"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
}

// sketchDocument is a sketch and its whole graph, for download.
type sketchDocument struct {
	Gosint  string             `json:"gosint"`
	Version int                `json:"version"`
	Sketch  sketchmodel.Sketch `json:"sketch"`
	Nodes   []graphmodel.Node  `json:"nodes"`
	Edges   []graphmodel.Edge  `json:"edges"`
	Files   []exportFile       `json:"files"`
}

// analysisDocument is one analysis inside an investigation export.
type analysisDocument struct {
	Title   string         `json:"title"`
	Content jsontext.Value `json:"content"`
}

// investigationDocument is an investigation with everything in it.
type investigationDocument struct {
	Gosint        string                 `json:"gosint"`
	Version       int                    `json:"version"`
	Investigation invmodel.Investigation `json:"investigation"`
	Sketches      []sketchDocument       `json:"sketches"`
	Analyses      []analysisDocument     `json:"analyses"`
}

// sketchImportResponse is the sketch an import made and what went in.
type sketchImportResponse struct {
	Sketch sketchmodel.Sketch `json:"sketch"`
	Result importResponse     `json:"result"`
}

// investigationImportResponse is the investigation an import made and
// the sum of what went in.
type investigationImportResponse struct {
	Investigation   invmodel.WithRole    `json:"investigation"`
	Sketches        []sketchmodel.Sketch `json:"sketches"`
	NodesCreated    int                  `json:"nodes_created"`
	EdgesCreated    int                  `json:"edges_created"`
	FilesCreated    int                  `json:"files_created"`
	AnalysesCreated int                  `json:"analyses_created"`
	Errors          []string             `json:"errors"`
}

// importFile is one file to attach, to the node the file's id names.
type importFile struct {
	ID          string `json:"id"           validate:"max=128"`
	Node        string `json:"node"         validate:"required,max=128"`
	Name        string `json:"name"         validate:"required,max=200"`
	ContentType string `json:"content_type" validate:"max=100"`
	Data        string `json:"data"         validate:"required"`
}

// importNode is one entity to import, as an export writes it.
type importNode struct {
	ID         string         `json:"id"         validate:"max=128"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Metadata   map[string]any `json:"metadata"`
	X          *float64       `json:"x"`
	Y          *float64       `json:"y"`

	// The style as our export carries it. Flag is one of the console's
	// flag colours, anything else is dropped.
	Color string `json:"color" validate:"max=32"`
	Icon  string `json:"icon"  validate:"max=128"`
	Image string `json:"image" validate:"max=4096"`
	Flag  string `json:"flag"  validate:"max=32"`
	Shape string `json:"shape" validate:"max=32"`
	Size  *int   `json:"size"  validate:"omitnil,min=1,max=1000"`
}

// importEdge links two imported entities by their ids in the file.
type importEdge struct {
	Source     string         `json:"source"     validate:"required,max=128"`
	Target     string         `json:"target"     validate:"required,max=128"`
	Label      string         `json:"label"      validate:"max=128" normalize:"trim"`
	Properties map[string]any `json:"properties"`
}

// importInput is a whole import.
type importInput struct {
	Nodes []importNode `json:"nodes" validate:"required,min=1,max=10000,dive"`
	Edges []importEdge `json:"edges" validate:"max=50000,dive"`
	Files []importFile `json:"files" validate:"max=1000,dive"`

	// Notes are said back first among the errors, what a report import
	// left out before it got here.
	Notes []string `json:"-"`

	// Source names the import's kind for the origin of what it adds:
	// file, nuclei, nmap, else import.
	Source string `json:"-"`
}

// importResponse says what an import did.
type importResponse struct {
	NodesCreated int      `json:"nodes_created"`
	NodesSkipped int      `json:"nodes_skipped"`
	EdgesCreated int      `json:"edges_created"`
	EdgesSkipped int      `json:"edges_skipped"`
	FilesCreated int      `json:"files_created"`
	FilesSkipped int      `json:"files_skipped"`
	Errors       []string `json:"errors"`
}
