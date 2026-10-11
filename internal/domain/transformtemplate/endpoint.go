// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
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
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
	tplmodel "github.com/yousysadmin/gosint/internal/models/transformtemplate"
)

// ScanKind is the kind of scan that runs a stored template.
const ScanKind = "template"

// testTimeout bounds the editor's test run of a template.
const testTimeout = time.Minute

// ErrOwnerOnly refuses running someone else's template that reads
// secrets. See the package doc.
var ErrOwnerOnly = errors.New("a shared template that reads secrets runs for its owner only")

// ErrNotFound is a template the runner cannot see.
var ErrNotFound = errors.New("template not found")

// Handler serves the templates.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the caller's templates and the public ones.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Template.Visible(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, templatesResponse{Templates: list})
}

// Get answers one visible template.
func (h Handler) Get(c fiber.Ctx) error {
	t, err := h.Runtime.Store.Template.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if t == nil {
		return response.NotFound(c, "template not found")
	}

	return response.Success(c, templateResponse{Template: *t})
}

// Create stores a new template.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	now := time.Now().UTC()
	t := &tplmodel.Template{ID: ids.New(), OwnerID: access.User(c).ID, CreatedAt: now}
	if resp, ok := h.fill(c, t, in, now); !ok {
		return resp
	}

	err := h.Runtime.Store.Template.Create(c, t)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "you already have a template named "+t.Name)
	}

	if err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, templateResponse{Template: *t})
}

// Update replaces one of the caller's templates.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	t, resp, ok := h.own(c)
	if !ok {
		return resp
	}

	if resp, ok := h.fill(c, t, in, time.Now().UTC()); !ok {
		return resp
	}

	err := h.Runtime.Store.Template.Update(c, t)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "you already have a template named "+t.Name)
	}

	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, templateResponse{Template: *t})
}

// Delete removes one of the caller's templates.
func (h Handler) Delete(c fiber.Ctx) error {
	if _, resp, ok := h.own(c); !ok {
		return resp
	}

	if _, err := h.Runtime.Store.Template.Delete(c, access.User(c).ID, c.Params("id")); err != nil {
		return response.Internal(c, err)
	}

	return response.NoContent(c)
}

// own loads the :id template when the caller owns it: 404 when they
// cannot see it, 403 when it is someone else's public one.
func (h Handler) own(c fiber.Ctx) (*tplmodel.Template, error, bool) {
	t, err := h.Runtime.Store.Template.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if t == nil {
		return nil, response.NotFound(c, "template not found"), false
	}

	if t.OwnerID != access.User(c).ID {
		return nil, response.Forbidden(c, "only its owner can change a template"), false
	}

	return t, nil, true
}

// fill parses the YAML into t. The name may not shadow a built-in.
func (h Handler) fill(c fiber.Ctx, t *tplmodel.Template, in saveInput, now time.Time) (error, bool) {
	parsed, err := template.Parse([]byte(in.Source), types.Builtin())
	if err != nil {
		return response.BadRequestFields(c, err.Error(),
			[]validation.FieldError{{Field: "source", Rule: "template", Message: err.Error()}}), false
	}

	if _, builtin := h.Runtime.Transforms.Get(parsed.Name); builtin {
		return response.Conflict(c, parsed.Name+" is the name of a built-in transform"), false
	}

	t.Name, t.Description, t.Category = parsed.Name, parsed.Description, parsed.Category
	t.InputType, t.OutputType, t.Source, t.Public, t.UpdatedAt = parsed.Input.Type, parsed.Output.Type, in.Source, in.Public, now

	return nil, true
}

// Load parses a stored template for runner, refusing one runner may not
// run. Used by the launch, the test and the worker.
func Load(ctx context.Context, rt *env.Runtime, runner, id string) (*template.Template, error) {
	t, err := rt.Store.Template.Get(ctx, runner, id)
	if err != nil {
		return nil, err
	}

	if t == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}

	parsed, err := template.Parse([]byte(t.Source), types.Builtin())
	if err != nil {
		return nil, err
	}

	if t.OwnerID != runner && len(parsed.Secrets) > 0 {
		return nil, ErrOwnerOnly
	}

	return parsed, nil
}

// Test runs the template on one value without writing anything.
func (h Handler) Test(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[testInput](c)
	if !ok {
		return resp
	}

	user := access.User(c).ID
	tpl, err := Load(c, h.Runtime, user, c.Params("id"))
	if resp, ok := refuseLoad(c, err); !ok {
		return resp
	}

	reg := types.Builtin()
	props := map[string]any{"label": in.Value}
	label, err := reg.Build(tpl.Input.Type, props)
	if err != nil {
		return response.BadRequest(c, "value is not a "+tpl.Input.Type+": "+err.Error())
	}

	// One request, bounded: a test run is the editor waiting, not a
	// scan with retries.
	e := template.Transform{T: tpl, NoRetry: true}
	params, resp, ok := h.params(c, e, user, in.Params)
	if !ok {
		return resp
	}

	ctx, cancel := context.WithTimeout(c, testTimeout)
	defer cancel()

	x := transform.NewExec(transform.ExecConfig{Log: discard{}, Params: params, Types: reg,
		HTTP: h.Runtime.TargetHTTP, Web: h.Runtime.TargetHTTP, Resolver: net.DefaultResolver})
	input := graphmodel.Node{Type: tpl.Input.Type, Label: label, Properties: props}
	outs, answer, err := e.Execute(ctx, x, input)

	out := testResponse{Answer: answer, Outputs: []testOutput{}}
	if err != nil {
		out.Error = err.Error()
	}

	for _, p := range outs {
		p = maps.Clone(p)
		if e.InPlace() {
			p = e.Complete(input, p)
		}

		o := testOutput{Properties: p}
		if o.Label, err = reg.Build(tpl.Output.Type, p); err != nil {
			o.Error = err.Error()
		}

		out.Outputs = append(out.Outputs, o)
	}

	return response.Success(c, out)
}

// Launch queues the template on nodes of the sketch. Behind
// requireSketch with update.
func (h Handler) Launch(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[launchInput](c)
	if !ok {
		return resp
	}

	user, sketchID, id := access.User(c).ID, access.Sketch(c).ID, c.Params("template_id")
	tpl, err := Load(c, h.Runtime, user, id)
	if resp, ok := refuseLoad(c, err); !ok {
		return resp
	}

	e := template.Transform{T: tpl, ID: id}
	if _, resp, ok := h.params(c, e, user, in.Params); !ok {
		return resp
	}

	usable, err := scan.UsableNodes(c, h.Runtime, sketchID, in.NodeIDs, tpl.Input.Type)
	if err != nil {
		return response.Internal(c, err)
	}

	if usable == 0 {
		return response.BadRequest(c, "none of the selected nodes is a "+tpl.Input.Type)
	}

	input := transform.Input{NodeIDs: in.NodeIDs, Params: in.Params}.InputMap()
	input["template_id"] = id
	sc := &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, OwnerID: user, Kind: ScanKind, Name: tpl.Name, Input: input}
	if err := scan.Enqueue(c, h.Runtime, sc); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, launchResponse{Scan: *sc})
}

// params resolves the template's secrets from the caller's vault.
func (h Handler) params(c fiber.Ctx, e template.Transform, user string, given map[string]string) (map[string]string, error, bool) {
	params, err := transform.ResolveParams(e.Meta().Params, given, func(ref string) (string, bool, error) {
		return vaultkey.Secret(c, h.Runtime, user, ref)
	})
	if pe, isParam := errors.AsType[*transform.ParamError](err); isParam {
		return nil, response.BadRequestFields(c, pe.Error(),
			[]validation.FieldError{{Field: "params." + pe.Name, Rule: "param", Message: pe.Error()}}), false
	}

	if err != nil {
		return nil, response.Internal(c, err), false
	}

	return params, nil, true
}

func refuseLoad(c fiber.Ctx, err error) (error, bool) {
	switch {
	case err == nil:
		return nil, true
	case errors.Is(err, ErrOwnerOnly):
		return response.Forbidden(c, err.Error()), false
	case errors.Is(err, ErrNotFound):
		return response.NotFound(c, "template not found"), false
	default:
		return response.Internal(c, err), false
	}
}

// Lookup is the worker's transform lookup for ScanKind: the scan's
// template, loaded for the scan's owner.
func Lookup(rt *env.Runtime) func(context.Context, *scanmodel.Scan) (transform.Transform, error) {
	return func(ctx context.Context, sc *scanmodel.Scan) (transform.Transform, error) {
		id, _ := sc.Input["template_id"].(string)
		tpl, err := Load(ctx, rt, sc.OwnerID, id)
		if err != nil {
			return nil, err
		}

		return template.Transform{T: tpl, ID: id}, nil
	}
}

type discard struct{}

func (discard) Log(logmodel.Level, string, map[string]any) {}
