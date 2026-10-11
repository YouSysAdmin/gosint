// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/response"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// The journal of hand edits. Every handler that changes the graph by
// hand takes a snapshot of what it touches before and after and
// records the pair, so undo writes the before back and redo the after.
// Transform runs and imports are not journaled: a run is undone by
// deleting what it found, a flow by its scans.

// snapNodes is the nodes among ids and every edge touching them.
func (h Handler) snapNodes(c fiber.Ctx, sketchID string, ids []string) (graphmodel.Snapshot, error) {
	var snap graphmodel.Snapshot
	if len(ids) == 0 {
		return snap, nil
	}

	nodes, err := h.Runtime.Store.Graph.NodesIn(c, sketchID, ids)
	if err != nil {
		return snap, err
	}

	edges, err := h.Runtime.Store.Graph.EdgesTouching(c, sketchID, ids)
	if err != nil {
		return snap, err
	}

	files, err := h.Runtime.Store.Attachment.ListForNodes(c, sketchID, ids)
	if err != nil {
		return snap, err
	}

	return graphmodel.Snapshot{Nodes: nodes, Edges: edges, Files: files}, nil
}

// snapEdges is the edges among ids.
func (h Handler) snapEdges(c fiber.Ctx, sketchID string, ids []string) (graphmodel.Snapshot, error) {
	edges, err := h.Runtime.Store.Graph.EdgesIn(c, sketchID, ids)

	return graphmodel.Snapshot{Edges: edges}, err
}

// snapGraph is the whole sketch.
func (h Handler) snapGraph(c fiber.Ctx, sketchID string) (graphmodel.Snapshot, error) {
	nodes, edges, err := h.Runtime.Store.Graph.Graph(c, sketchID)
	if err != nil {
		return graphmodel.Snapshot{}, err
	}

	files, err := h.Runtime.Store.Attachment.ListForSketch(c, sketchID)

	return graphmodel.Snapshot{Nodes: nodes, Edges: edges, Files: files}, err
}

// changed tells the sketch's live streams that the caller changed the
// graph, so every other member's console reloads it.
func (h Handler) changed(c fiber.Ctx, sketchID, kind string) {
	u := access.User(c)
	h.Runtime.Events.Publish(eventbus.Event{Type: eventbus.TypeGraph, SketchID: sketchID,
		Data: eventbus.GraphChange{By: u.ID, Name: u.DisplayName(), Kind: kind}, At: time.Now().UTC()})
}

// record journals one edit by the caller. A failure to journal is
// logged, not answered: the edit itself went through.
func (h Handler) record(c fiber.Ctx, sketchID, kind string, before, after graphmodel.Snapshot) {
	ch := &graphmodel.Change{SketchID: sketchID, UserID: access.User(c).ID, Kind: kind, Before: before, After: after}
	if err := h.Runtime.Store.Graph.RecordChange(c, ch); err != nil {
		h.Runtime.Log.Warn("journal an edit", "sketch", sketchID, "kind", kind, "err", err)
	}

	h.changed(c, sketchID, kind)
}

// counted reads "3 entities" or "1 relationship".
func counted(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}

	return fmt.Sprintf("%d %s", n, many)
}

// changesResponse says what Undo and Redo would do next, "" for
// nothing.
type changesResponse struct {
	Undo string `json:"undo"`
	Redo string `json:"redo"`
}

// revertResponse answers an undo or a redo: what was reverted and what
// the two buttons do next.
type revertResponse struct {
	Reverted string `json:"reverted"`
	Undo     string `json:"undo"`
	Redo     string `json:"redo"`
}

func (h Handler) next(c fiber.Ctx, sketchID string) (changesResponse, error) {
	user := access.User(c).ID
	var out changesResponse
	if ch, err := h.Runtime.Store.Graph.NextUndo(c, sketchID, user); err != nil {
		return out, err
	} else if ch != nil {
		out.Undo = ch.Kind
	}

	if ch, err := h.Runtime.Store.Graph.NextRedo(c, sketchID, user); err != nil {
		return out, err
	} else if ch != nil {
		out.Redo = ch.Kind
	}

	return out, nil
}

// Changes says what the caller can undo and redo on the sketch.
func (h Handler) Changes(c fiber.Ctx) error {
	out, err := h.next(c, h.sketchID(c))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, out)
}

// Undo reverts the caller's newest edit of the sketch still in effect.
func (h Handler) Undo(c fiber.Ctx) error {
	return h.revert(c, true)
}

// Redo applies the caller's last undone edit again.
func (h Handler) Redo(c fiber.Ctx) error {
	return h.revert(c, false)
}

func (h Handler) revert(c fiber.Ctx, undo bool) error {
	sketchID, user := h.sketchID(c), access.User(c).ID
	find := h.Runtime.Store.Graph.NextRedo
	if undo {
		find = h.Runtime.Store.Graph.NextUndo
	}

	ch, err := find(c, sketchID, user)
	if err != nil {
		return response.Internal(c, err)
	}

	if ch == nil {
		return response.BadRequest(c, "nothing to "+verb(undo))
	}

	from, to := ch.After, ch.Before
	if !undo {
		from, to = ch.Before, ch.After
	}

	if err := h.Runtime.Store.Graph.Revert(c, sketchID, from, to); err != nil {
		return response.Internal(c, err)
	}

	if err := h.Runtime.Store.Graph.SetUndone(c, ch.ID, undo); err != nil {
		return response.Internal(c, err)
	}

	next, err := h.next(c, sketchID)
	if err != nil {
		return response.Internal(c, err)
	}

	h.changed(c, sketchID, verb(undo)+" "+ch.Kind)

	return response.Success(c, revertResponse{Reverted: ch.Kind, Undo: next.Undo, Redo: next.Redo})
}

// verb names the move: undo or redo.
func verb(undo bool) string {
	if undo {
		return "undo"
	}

	return "redo"
}
