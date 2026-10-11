// Gosint, Copyright (c) 2026 YouSysAdmin

// Package graph persists the nodes and edges of sketches and serves the
// graph endpoints.
//
// The graph is two tables keyed by sketch, no graph database: load a
// whole sketch, read one hop around a node, and upsert entities by
// (type, key) so a transform finding one again enriches the same node.
package graph
