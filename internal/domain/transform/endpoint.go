// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"errors"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/template"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// Handler serves the transform catalog and launches.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the built-in transforms and the templates the caller may
// see, those taking ?type= when given.
func (h Handler) List(c fiber.Ctx) error {
	typ := c.Query("type")
	list := h.Runtime.Transforms.For(typ)
	out := make([]listed, 0, len(list))
	for _, e := range list {
		l := listed{Meta: e.Meta()}
		if f, ok := e.(interface{ File() string }); ok {
			l.File = f.File()
		}

		out = append(out, l)
	}

	tpls, err := h.Runtime.Store.Template.Visible(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	for _, t := range tpls {
		if typ != "" && t.InputType != typ {
			continue
		}

		meta, err := templateMeta(t.ID, t.Source, t.UpdatedAt)
		if err != nil {
			// A stored template the parser no longer takes is left out of
			// the catalog, its editor shows the refusal.
			h.Runtime.Log.Warn("transform catalog: template does not parse", "template", t.ID, "err", err)

			continue
		}

		out = append(out, listed{Meta: meta, TemplateID: t.ID, Public: t.Public})
	}

	return response.Success(c, transformsResponse{Transforms: out})
}

// Launch queues a transform run on nodes of the sketch. Behind
// requireSketch with update, since the run writes into the sketch.
//
// The parameters are resolved here as well as at run time, so a missing
// secret is a 400 the caller sees rather than a failed scan later.
func (h Handler) Launch(c fiber.Ctx) error {
	e, ok := h.Runtime.Transforms.Get(c.Params("name"))
	if !ok {
		return response.NotFound(c, "no such transform")
	}

	in, resp, ok := validation.Bind[launchInput](c)
	if !ok {
		return resp
	}

	meta, sketchID, owner := e.Meta(), access.Sketch(c).ID, access.User(c).ID
	_, err := transform.ResolveParams(meta.Params, in.Params, func(ref string) (string, bool, error) {
		return vaultkey.Secret(c, h.Runtime, owner, ref)
	})
	if pe, isParam := errors.AsType[*transform.ParamError](err); isParam {
		return response.BadRequestFields(c, pe.Error(),
			[]validation.FieldError{{Field: "params." + pe.Name, Rule: "param", Message: pe.Error()}})
	}

	if err != nil {
		return response.Internal(c, err)
	}

	usable, err := scan.UsableNodes(c, h.Runtime, sketchID, in.NodeIDs, meta.Input)
	if err != nil {
		return response.Internal(c, err)
	}

	if usable == 0 {
		return response.BadRequest(c, "none of the selected nodes is a "+meta.Input)
	}

	sc := &scanmodel.Scan{
		ID:       ids.New(),
		SketchID: sketchID,
		OwnerID:  owner,
		Kind:     transform.ScanKind,
		Name:     meta.Name,
		Input:    transform.Input{NodeIDs: in.NodeIDs, Params: in.Params}.InputMap(),
	}
	if err := scan.Enqueue(c, h.Runtime, sc); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, launchResponse{Scan: *sc})
}

// parsedMetas keeps each template's Meta by id and edit time, so the
// catalog parses a template again only once it changed.
var parsedMetas sync.Map

type parsedMeta struct {
	at   time.Time
	meta transform.Meta
}

// templateMeta is the Meta of a stored template, parsed once per edit.
func templateMeta(id, source string, updated time.Time) (transform.Meta, error) {
	if v, ok := parsedMetas.Load(id); ok {
		if p, ok := v.(parsedMeta); ok && p.at.Equal(updated) {
			return p.meta, nil
		}
	}

	parsed, err := template.Parse([]byte(source), types.Builtin())
	if err != nil {
		return transform.Meta{}, err
	}

	meta := template.Transform{T: parsed}.Meta()
	parsedMetas.Store(id, parsedMeta{at: updated, meta: meta})

	return meta, nil
}
