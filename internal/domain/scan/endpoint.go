// Gosint, Copyright (c) 2026 YouSysAdmin

package scan

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/paging"
	"github.com/yousysadmin/gosint/internal/core/response"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// Handler serves a sketch's scans. Every route is behind requireSketch.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the sketch's newest scans.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Scan.ListBySketch(c, access.Sketch(c).ID, paging.Limit(c, 50, 500))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, scansResponse{Scans: list})
}

// load returns the :scan_id scan when it belongs to the sketch.
func (h Handler) load(c fiber.Ctx) (*scanmodel.Scan, error) {
	sc, err := h.Runtime.Store.Scan.Get(c, c.Params("scan_id"))
	if err != nil || sc == nil || sc.SketchID != access.Sketch(c).ID {
		return nil, err
	}

	return sc, nil
}

// Get answers one scan.
func (h Handler) Get(c fiber.Ctx) error {
	sc, err := h.load(c)
	if err != nil {
		return response.Internal(c, err)
	}

	if sc == nil {
		return response.NotFound(c, "scan not found")
	}

	return response.Success(c, scanResponse{Scan: *sc})
}

// Cancel stops a scan that has not finished.
func (h Handler) Cancel(c fiber.Ctx) error {
	sc, err := h.load(c)
	if err != nil {
		return response.Internal(c, err)
	}

	if sc == nil {
		return response.NotFound(c, "scan not found")
	}

	now := time.Now().UTC()
	stopped, err := h.Runtime.Store.Scan.Cancel(c, sc.ID, now)
	if err != nil {
		return response.Internal(c, err)
	}

	if !stopped {
		return response.Conflict(c, "the scan already finished")
	}

	sc.Status, sc.CompletedAt = scanmodel.StatusCancelled, &now
	h.Runtime.Events.Publish(eventbus.Event{Type: eventbus.TypeScan, SketchID: sc.SketchID, Data: *sc})

	return response.Success(c, scanResponse{Scan: *sc})
}

// Delete removes a finished scan from the history.
func (h Handler) Delete(c fiber.Ctx) error {
	sc, err := h.load(c)
	if err != nil {
		return response.Internal(c, err)
	}

	if sc == nil {
		return response.NotFound(c, "scan not found")
	}

	if !sc.Status.Final() {
		return response.Conflict(c, "cancel the scan before deleting it")
	}

	if err := h.Runtime.Store.Scan.Delete(c, sc.ID); err != nil {
		return response.Internal(c, err)
	}

	return response.NoContent(c)
}

// Enqueue queues a scan and tells the worker. For the endpoints that
// launch transforms and flows.
func Enqueue(ctx context.Context, rt *env.Runtime, sc *scanmodel.Scan) error {
	sc.Status = scanmodel.StatusPending
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = time.Now().UTC()
	}

	if err := rt.Store.Scan.Enqueue(ctx, sc); err != nil {
		return err
	}

	if sc.SketchID != "" {
		rt.Events.Publish(eventbus.Event{Type: eventbus.TypeScan, SketchID: sc.SketchID, Data: *sc})
	}

	if rt.WakeWorker != nil {
		rt.WakeWorker()
	}

	return nil
}

// UsableNodes counts the selected nodes a transform of inputType can
// take, "" taking any. A launch with none to work on is refused before
// a scan is queued, the same way for a transform, a flow and a template.
func UsableNodes(ctx context.Context, rt *env.Runtime, sketchID string, ids []string, inputType string) (int, error) {
	nodes, err := rt.Store.Graph.NodesIn(ctx, sketchID, ids)
	if err != nil {
		return 0, err
	}

	usable := 0
	for _, n := range nodes {
		if inputType == "" || n.Type == inputType {
			usable++
		}
	}

	return usable, nil
}
