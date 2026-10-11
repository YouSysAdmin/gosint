// Gosint, Copyright (c) 2026 YouSysAdmin

package entitytype

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
)

// Fallback is what a value nothing recognizes becomes.
const Fallback = "phrase"

// Handler serves the type catalog.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the palette groups and every entity type with its fields.
func (h Handler) List(c fiber.Ctx) error {
	cats := make([]categoryView, 0, len(types.Categories))
	for _, cat := range types.Categories {
		cats = append(cats, categoryView{Key: cat.Key, Title: cat.Title, Icon: cat.Icon})
	}

	all := types.Builtin().All()
	out := make([]typeView, 0, len(all))
	for _, t := range all {
		fields := make([]fieldView, 0, len(t.Fields))
		for _, f := range t.Fields {
			fields = append(fields, fieldView{
				Name: f.Name, Title: f.Title, Description: f.Description, Kind: string(f.Kind),
				Elem: string(f.Elem), Ref: f.Ref, Required: f.Required, Primary: f.Primary, Long: f.Long, Default: f.Default,
				Options: f.Options, Min: f.Min, Max: f.Max, VaultKey: f.VaultKey, Secret: f.Secret,
			})
		}

		out = append(out, typeView{
			Name: t.Name, Title: t.Title, Description: t.Description, Category: t.Category,
			Icon: t.Icon, Color: t.Color, Detectable: t.Detect != nil, Fields: fields,
		})
	}

	return response.Success(c, typesResponse{Categories: cats, Types: out})
}

// Detect recognizes each value's entity type and parses it.
func (h Handler) Detect(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[detectInput](c)
	if !ok {
		return resp
	}

	reg := types.Builtin()
	out := make([]detection, 0, len(in.Values))
	for _, raw := range in.Values {
		v := strings.TrimSpace(raw)
		d := detection{Value: v, Type: Fallback, Label: v, Properties: map[string]any{"text": v}}

		if t, found := reg.Detect(v); found {
			_, props, err := reg.Parse(v)
			if err != nil {
				d.Type, d.Error = t.Name, err.Error()
			} else {
				d.Type, d.Properties, d.Label = t.Name, props, t.Label(props)
			}
		}

		out = append(out, d)
	}

	return response.Success(c, detectResponse{Results: out})
}

// Refuse answers a type refusal from core/types as a 400 naming the
// property, in the envelope validation.Bind uses.
func Refuse(c fiber.Ctx, err error) error {
	field, msg := "properties", err.Error()
	if fe, ok := errors.AsType[*types.FieldError](err); ok {
		field, msg = "properties."+fe.Field, fe.Field+" "+fe.Message
	}

	if errors.Is(err, types.ErrNoLabel) {
		field, msg = "label", "a label or the type's main field is required"
	}

	return response.BadRequestFields(c, msg, []validation.FieldError{{Field: field, Rule: "type", Message: msg}})
}
