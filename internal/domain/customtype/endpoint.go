// Gosint, Copyright (c) 2026 YouSysAdmin

package customtype

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	ctmodel "github.com/yousysadmin/gosint/internal/models/customtype"
)

// Handler serves the declarations to the admin panel.
type Handler struct {
	Runtime *env.Runtime
}

// saveInput declares or changes a type. The name is fixed once made.
type saveInput struct {
	Name        string              `json:"name"        validate:"required,max=41" normalize:"trim,lower"`
	Title       string              `json:"title"       validate:"required,notblank,max=100" normalize:"trim"`
	Description string              `json:"description" validate:"max=2000" normalize:"trim"`
	Category    string              `json:"category"    validate:"required,max=40" normalize:"trim"`
	Icon        string              `json:"icon"        validate:"max=60" normalize:"trim"`
	Color       string              `json:"color"       validate:"max=7" normalize:"trim"`
	Fields      []types.CustomField `json:"fields"      validate:"required,min=1,max=64"`
}

type listResponse struct {
	Types []ctmodel.CustomType `json:"types"`
}

type oneResponse struct {
	Type ctmodel.CustomType `json:"type"`
}

// Load reads every declaration into the registry: at boot and by the
// refresh job, so a change on one node reaches the others.
func Load(ctx context.Context, rt *env.Runtime) error {
	all, err := rt.Store.CustomType.List(ctx)
	if err != nil {
		return err
	}

	defs := make([]types.CustomType, 0, len(all))
	for _, t := range all {
		defs = append(defs, t.CustomType)
	}

	return types.Builtin().SetCustom(defs)
}

// List answers every declaration.
func (h Handler) List(c fiber.Ctx) error {
	all, err := h.Runtime.Store.CustomType.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, listResponse{Types: all})
}

func (h Handler) check(c fiber.Ctx, def types.CustomType) (error, bool) {
	if err := types.Builtin().CheckCustom(def); err != nil {
		return response.BadRequestFields(c, err.Error(),
			[]validation.FieldError{{Field: "fields", Rule: "type", Message: err.Error()}}), false
	}

	return nil, true
}

func (in saveInput) def() types.CustomType {
	fields := make([]types.CustomField, 0, len(in.Fields))
	for _, f := range in.Fields {
		f.Name = strings.ToLower(strings.TrimSpace(f.Name))
		f.Title = strings.TrimSpace(f.Title)
		fields = append(fields, f)
	}

	return types.CustomType{Name: in.Name, Title: in.Title, Description: in.Description, Category: in.Category,
		Icon: in.Icon, Color: in.Color, Fields: fields}
}

// Create declares a type and puts it in the registry at once.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	def := in.def()
	if resp, ok := h.check(c, def); !ok {
		return resp
	}

	now := time.Now().UTC()
	t := &ctmodel.CustomType{ID: ids.New(), CreatedBy: access.User(c).ID, CreatedAt: now, UpdatedAt: now, CustomType: def}
	err := h.Runtime.Store.CustomType.Create(c, t)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "a type named "+t.Name+" exists already")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	if err := Load(c, h.Runtime); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, oneResponse{Type: *t})
}

// Update changes a declaration, its name kept.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	t, err := h.Runtime.Store.CustomType.Get(c, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if t == nil {
		return response.NotFound(c, "type not found")
	}

	def := in.def()
	def.Name = t.Name
	if resp, ok := h.check(c, def); !ok {
		return resp
	}

	t.CustomType, t.UpdatedAt = def, time.Now().UTC()
	if err := h.Runtime.Store.CustomType.Update(c, t); err != nil {
		return response.Internal(c, err)
	}

	if err := Load(c, h.Runtime); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, oneResponse{Type: *t})
}

// Delete removes a declaration nothing uses: a type with entities in
// any sketch stays, and says how many.
func (h Handler) Delete(c fiber.Ctx) error {
	t, err := h.Runtime.Store.CustomType.Get(c, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if t == nil {
		return response.NotFound(c, "type not found")
	}

	n, err := h.Runtime.Store.CustomType.CountNodes(c, t.Name)
	if err != nil {
		return response.Internal(c, err)
	}

	if n > 0 {
		return response.Conflict(c, fmt.Sprintf("%d entities are of this type, delete them first", n))
	}

	if _, err := h.Runtime.Store.CustomType.Delete(c, t.ID); err != nil {
		return response.Internal(c, err)
	}

	if err := Load(c, h.Runtime); err != nil {
		return response.Internal(c, err)
	}

	return response.NoContent(c)
}
