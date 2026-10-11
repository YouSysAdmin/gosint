// Gosint, Copyright (c) 2026 YouSysAdmin

package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// Handler serves the caller's flows.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the caller's flows, those starting from ?type= if given.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Flow.List(c, access.User(c).ID, c.Query("type"))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, flowsResponse{Flows: list})
}

// Get answers one of the caller's flows.
func (h Handler) Get(c fiber.Ctx) error {
	f, err := h.Runtime.Store.Flow.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if f == nil {
		return response.NotFound(c, "flow not found")
	}

	return response.Success(c, flowResponse{Flow: *f})
}

// Check answers the steps of a definition in run order, or why it
// cannot run, for the editor.
func (h Handler) Check(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[checkInput](c)
	if !ok {
		return resp
	}

	if resp, ok := h.refuse(c, in.Definition); !ok {
		return resp
	}

	steps, _ := coreflow.Order(in.Definition)
	out := make([]plannedStep, 0, len(steps))
	for _, s := range steps {
		e, ok := h.Runtime.Transforms.Get(s.Transform)
		if !ok {
			continue
		}

		m := e.Meta()
		out = append(out, plannedStep{ID: s.ID, After: s.After, Transform: s.Transform, Input: m.Input, Output: m.Output})
	}

	return response.Success(c, checkResponse{Steps: out})
}

// Create stores a new flow.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	if resp, ok := h.refuse(c, in.Definition); !ok {
		return resp
	}

	now := time.Now().UTC()
	f := &flowmodel.Flow{ID: ids.New(), OwnerID: access.User(c).ID, Name: in.Name, Description: in.Description,
		Definition: in.Definition, CreatedAt: now, UpdatedAt: now}

	err := h.Runtime.Store.Flow.Create(c, f)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "you already have a flow named "+f.Name)
	}

	if err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, flowResponse{Flow: *f})
}

// Update replaces one of the caller's flows.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	f, err := h.Runtime.Store.Flow.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if f == nil {
		return response.NotFound(c, "flow not found")
	}

	if resp, ok := h.refuse(c, in.Definition); !ok {
		return resp
	}

	f.Name, f.Description, f.Definition, f.UpdatedAt = in.Name, in.Description, in.Definition, time.Now().UTC()

	err = h.Runtime.Store.Flow.Update(c, f)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "you already have a flow named "+f.Name)
	}

	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, flowResponse{Flow: *f})
}

// Delete removes one of the caller's flows.
func (h Handler) Delete(c fiber.Ctx) error {
	found, err := h.Runtime.Store.Flow.Delete(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !found {
		return response.NotFound(c, "flow not found")
	}

	return response.NoContent(c)
}

// Launch queues the flow on nodes of the sketch. Behind requireSketch
// with update.
func (h Handler) Launch(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[launchInput](c)
	if !ok {
		return resp
	}

	user, sketchID := access.User(c).ID, access.Sketch(c).ID
	f, err := h.Runtime.Store.Flow.Get(c, user, c.Params("flow_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if f == nil {
		return response.NotFound(c, "flow not found")
	}

	usable, err := scan.UsableNodes(c, h.Runtime, sketchID, in.NodeIDs, f.Definition.Input)
	if err != nil {
		return response.Internal(c, err)
	}

	if usable == 0 {
		return response.BadRequest(c, "none of the selected nodes is a "+f.Definition.Input)
	}

	nodeIDs := make([]any, 0, len(in.NodeIDs))
	for _, id := range in.NodeIDs {
		nodeIDs = append(nodeIDs, id)
	}

	sc := &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, OwnerID: user, Kind: coreflow.ScanKind, Name: f.Name,
		Input: map[string]any{"flow_id": f.ID, "node_ids": nodeIDs}}
	if err := scan.Enqueue(c, h.Runtime, sc); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, launchResponse{Scan: *sc})
}

// InlineScan is a scan running a definition composed for this one run
// and never stored, the chat's: the definition travels in the scan's
// input in place of a flow id. It must have passed coreflow.Check.
func InlineScan(sketchID, owner string, def flowmodel.Definition, nodeIDs []string) *scanmodel.Scan {
	list := make([]any, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		list = append(list, id)
	}

	def.Layout = nil
	steps, _ := coreflow.Order(def)
	names := make([]string, 0, len(steps))
	for _, s := range steps {
		names = append(names, s.Transform)
	}

	return &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, OwnerID: owner, Kind: coreflow.ScanKind,
		Name: "inline: " + strings.Join(names, ", "), Input: map[string]any{"definition": def, "node_ids": list}}
}

// refuse answers a definition that cannot run as a 400 on definition.
func (h Handler) refuse(c fiber.Ctx, d flowmodel.Definition) (error, bool) {
	if err := coreflow.Check(d, h.Runtime.Transforms); err != nil {
		return response.BadRequestFields(c, err.Error(),
			[]validation.FieldError{{Field: "definition", Rule: "flow", Message: err.Error()}}), false
	}

	return nil, true
}

// ErrNotFound is a flow id the owner has not got.
var ErrNotFound = errors.New("flow not found")

// Definition loads a flow for the worker, for its owner.
func Definition(rt *env.Runtime) func(ctx context.Context, flowID, ownerID string) (*flowmodel.Definition, error) {
	return func(ctx context.Context, flowID, ownerID string) (*flowmodel.Definition, error) {
		f, err := rt.Store.Flow.Get(ctx, ownerID, flowID)
		if err != nil {
			return nil, err
		}

		if f == nil {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, flowID)
		}

		return &f.Definition, nil
	}
}
