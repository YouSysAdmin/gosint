// Gosint, Copyright (c) 2026 YouSysAdmin

// Package analysis is the written analyses of an investigation, notes
// that reference the entities of its sketches.
package analysis

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	amodel "github.com/yousysadmin/gosint/internal/models/analysis"
)

// Handler serves analyses. Access is the investigation's.
type Handler struct {
	Runtime *env.Runtime
}

// content checks the document is a JSON object within bounds, an
// empty one standing in for none.
func content(raw jsontext.Value) (jsontext.Value, bool) {
	if len(raw) == 0 {
		return jsontext.Value("{}"), true
	}

	if len(raw) > maxContent {
		return nil, false
	}

	var probe map[string]any

	return raw, json.Unmarshal(raw, &probe) == nil
}

// List answers the investigation's analyses, without content.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Analysis.List(c, access.Investigation(c).Investigation.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if list == nil {
		list = []amodel.Analysis{}
	}

	return response.Success(c, analysesResponse{Analyses: list})
}

// Create starts an analysis.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	doc, ok := content(in.Content)
	if !ok {
		return response.BadRequest(c, "content must be a JSON document under 2 MB")
	}

	now := time.Now().UTC()
	a := &amodel.Analysis{ID: ids.New(), InvestigationID: access.Investigation(c).Investigation.ID,
		OwnerID: access.User(c).ID, Title: in.Title, Content: doc, CreatedAt: now, UpdatedAt: now}
	if err := h.Runtime.Store.Analysis.Create(c, a); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, analysisResponse{Analysis: *a})
}

// Get answers one analysis with its content.
func (h Handler) Get(c fiber.Ctx) error {
	a, err := h.Runtime.Store.Analysis.Get(c, access.Investigation(c).Investigation.ID, c.Params("analysis_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if a == nil {
		return response.NotFound(c, "analysis not found")
	}

	return response.Success(c, analysisResponse{Analysis: *a})
}

// Update rewrites title and content.
func (h Handler) Update(c fiber.Ctx) error {
	a, err := h.Runtime.Store.Analysis.Get(c, access.Investigation(c).Investigation.ID, c.Params("analysis_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if a == nil {
		return response.NotFound(c, "analysis not found")
	}

	in, resp, ok := validation.BindOnto(c, saveInput{Title: a.Title, Content: a.Content})
	if !ok {
		return resp
	}

	doc, ok := content(in.Content)
	if !ok {
		return response.BadRequest(c, "content must be a JSON document under 2 MB")
	}

	a.Title, a.Content, a.UpdatedAt = in.Title, doc, time.Now().UTC()
	if err := h.Runtime.Store.Analysis.Update(c, a); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, analysisResponse{Analysis: *a})
}

// Delete removes an analysis.
func (h Handler) Delete(c fiber.Ctx) error {
	gone, err := h.Runtime.Store.Analysis.Delete(c, access.Investigation(c).Investigation.ID, c.Params("analysis_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !gone {
		return response.NotFound(c, "analysis not found")
	}

	return response.NoContent(c)
}
