// Gosint, Copyright (c) 2026 YouSysAdmin

package sketch

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
)

// Handler serves the sketch endpoints. Create sits behind
// requireInvestigation, the :id routes behind requireSketch.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the sketches of every investigation the caller is on.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Sketch.ListForUser(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, sketchesResponse{Sketches: list})
}

// ListByInvestigation answers the sketches of the investigation.
func (h Handler) ListByInvestigation(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Sketch.ListByInvestigation(c, access.Investigation(c).Investigation.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, sketchesResponse{Sketches: list})
}

// Create adds a sketch to the investigation.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[createInput](c)
	if !ok {
		return resp
	}

	now := time.Now().UTC()
	sk := &sketchmodel.Sketch{
		ID:              ids.New(),
		InvestigationID: access.Investigation(c).Investigation.ID,
		Title:           in.Title,
		Description:     in.Description,
		Status:          "active",
		OwnerID:         access.User(c).ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.Runtime.Store.Sketch.Create(c, sk); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, sketchResponse{Sketch: *sk})
}

// Get answers one sketch.
func (h Handler) Get(c fiber.Ctx) error {
	return response.Success(c, sketchResponse{Sketch: *access.Sketch(c)})
}

// Update changes title, description or status.
func (h Handler) Update(c fiber.Ctx) error {
	sk := *access.Sketch(c)
	in, resp, ok := validation.BindOnto(c, updateInput{Title: sk.Title, Description: sk.Description, Status: sk.Status})
	if !ok {
		return resp
	}

	sk.Title, sk.Description, sk.Status = in.Title, in.Description, in.Status
	sk.UpdatedAt = time.Now().UTC()
	if err := h.Runtime.Store.Sketch.Update(c, &sk); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, sketchResponse{Sketch: sk})
}

// Delete removes the sketch and its graph.
func (h Handler) Delete(c fiber.Ctx) error {
	hashes, err := h.Runtime.Store.Attachment.HashesOfSketch(c, access.Sketch(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if err := h.Runtime.Store.Sketch.Delete(c, access.Sketch(c).ID); err != nil {
		return response.Internal(c, err)
	}

	attachment.Release(c, h.Runtime, hashes)

	return response.NoContent(c)
}
