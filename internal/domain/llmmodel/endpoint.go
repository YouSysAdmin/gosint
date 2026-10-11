// Gosint, Copyright (c) 2026 YouSysAdmin

package llmmodel

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	model "github.com/yousysadmin/gosint/internal/models/llmmodel"
)

// DefaultKeyName is the secret a new model asks each account for.
const DefaultKeyName = "LLM_API_KEY"

// ErrNoModel is an install with no model to talk to.
var ErrNoModel = errors.New("no language model is set up, an administrator adds one under Administration, Models")

// Handler serves the models: the admin's list and the picker's.
type Handler struct {
	Runtime *env.Runtime
}

// Resolve is the model a person asked for, else the default one, else
// the first, so a model deleted since the pick still answers.
func Resolve(ctx context.Context, rt *env.Runtime, id string) (*model.Model, error) {
	if id != "" {
		m, err := rt.Store.LLMModel.Get(ctx, id)
		if err != nil || m != nil {
			return m, err
		}
	}

	all, err := rt.Store.LLMModel.List(ctx)
	if err != nil {
		return nil, err
	}

	if len(all) == 0 {
		return nil, ErrNoModel
	}

	for i := range all {
		if all[i].Default {
			return &all[i], nil
		}
	}

	return &all[0], nil
}

func adminView(m *model.Model) AdminView {
	return AdminView{ID: m.ID, Name: m.Name, API: m.API, BaseURL: m.BaseURL, Model: m.Model, Tools: m.Tools,
		KeyName: m.KeyName, HasAPIKey: m.APIKey != "", Default: m.Default, Local: m.Local(),
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

// Pick lists the models a person may pick for the assistant.
func (h Handler) Pick(c fiber.Ctx) error {
	all, err := h.Runtime.Store.LLMModel.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	out := pickListResponse{Models: make([]PickView, 0, len(all))}
	for _, m := range all {
		out.Models = append(out.Models, PickView{ID: m.ID, Name: m.Name, Model: m.Model, Default: m.Default,
			KeyName: m.KeyName, NeedsKey: m.APIKey == "" && !m.Local()})
	}

	return response.Success(c, out)
}

// List answers every model to the admin panel.
func (h Handler) List(c fiber.Ctx) error {
	all, err := h.Runtime.Store.LLMModel.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	out := adminListResponse{Models: make([]AdminView, 0, len(all))}
	for i := range all {
		out.Models = append(out.Models, adminView(&all[i]))
	}

	return response.Success(c, out)
}

// Create adds a model. The first one is the default.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchInput](c)
	if !ok {
		return resp
	}

	all, err := h.Runtime.Store.LLMModel.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	m := &model.Model{ID: ids.New(), API: llm.APIOpenAI, Tools: model.ToolsAuto, KeyName: DefaultKeyName,
		Default: len(all) == 0}
	if resp, ok := apply(c, m, in); !ok {
		return resp
	}

	if err := h.Runtime.Store.LLMModel.Put(c, m); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, adminOneResponse{Model: adminView(m)})
}

// Update patches a model.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchInput](c)
	if !ok {
		return resp
	}

	m, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	// The default moves by naming another one, never by switching it off.
	if in.Default != nil && !*in.Default && m.Default {
		in.Default = nil
	}

	if resp, ok := apply(c, m, in); !ok {
		return resp
	}

	if err := h.Runtime.Store.LLMModel.Put(c, m); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, adminOneResponse{Model: adminView(m)})
}

// Delete removes a model. Deleting the default hands the switch to the
// first one left.
func (h Handler) Delete(c fiber.Ctx) error {
	m, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	if _, err := h.Runtime.Store.LLMModel.Delete(c, m.ID); err != nil {
		return response.Internal(c, err)
	}

	if m.Default {
		rest, err := h.Runtime.Store.LLMModel.List(c)
		if err != nil {
			return response.Internal(c, err)
		}

		if len(rest) > 0 {
			rest[0].Default = true
			if err := h.Runtime.Store.LLMModel.Put(c, &rest[0]); err != nil {
				return response.Internal(c, err)
			}
		}
	}

	return response.NoContent(c)
}

func (h Handler) load(c fiber.Ctx) (*model.Model, error, bool) {
	m, err := h.Runtime.Store.LLMModel.Get(c, c.Params("id"))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if m == nil {
		return nil, response.NotFound(c, "model not found"), false
	}

	return m, nil, true
}

// apply folds the input onto the model and checks the whole.
func apply(c fiber.Ctx, m *model.Model, in patchInput) (error, bool) {
	set := func(dst, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}

	set(&m.Name, in.Name)
	set(&m.API, in.API)
	set(&m.BaseURL, in.BaseURL)
	set(&m.Model, in.Model)
	set(&m.Tools, in.Tools)
	set(&m.KeyName, in.KeyName)
	set(&m.APIKey, in.APIKey)
	if in.Default != nil {
		m.Default = *in.Default
	}

	if m.Name == "" {
		return refuse(c, "name", "required", "name must not be empty"), false
	}

	if u, err := url.Parse(m.BaseURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return refuse(c, "base_url", "url", "base_url must be an http(s) URL"), false
	}

	if m.Model == "" && !m.Local() {
		return refuse(c, "model", "required", "a model served elsewhere than this machine needs its name"), false
	}

	return nil, true
}

func refuse(c fiber.Ctx, field, rule, msg string) error {
	return response.BadRequestFields(c, msg, []validation.FieldError{{Field: field, Rule: rule, Message: msg}})
}
