// Gosint, Copyright (c) 2026 YouSysAdmin

package schedule

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
	smodel "github.com/yousysadmin/gosint/internal/models/schedule"
)

// Handler serves a sketch's schedules, behind the sketch's guards.
type Handler struct {
	Runtime *env.Runtime
}

// maxInputs bounds what one scheduled run is handed. The interval is
// five minutes to thirty days, on the input's validation.
const maxInputs = 1000

// saveInput creates or changes a schedule. NodeIDs empty means every
// entity of the flow's input type at run time.
type saveInput struct {
	FlowID  string   `json:"flow_id"  validate:"omitempty,uuid"`
	NodeIDs []string `json:"node_ids" validate:"max=1000,dive,uuid"`
	Every   int      `json:"every_seconds" validate:"required,min=300,max=2592000"`
	Enabled *bool    `json:"enabled"`
}

// Summary is what the last run found, read off the graph's provenance:
// entities the scan created, ones it found again, and ones the run
// before saw that this one did not.
type Summary struct {
	ScanID  string `json:"scan_id"`
	Status  string `json:"status"`
	New     int    `json:"new"`
	Seen    int    `json:"seen"`
	Missing int    `json:"missing"`
}

// listed is a schedule with its last run summed up.
type listed struct {
	smodel.Schedule
	Summary *Summary `json:"summary"`
}

type listResponse struct {
	Schedules []listed `json:"schedules"`
}

type oneResponse struct {
	Schedule listed `json:"schedule"`
}

type runResponse struct {
	Scan scanmodel.Scan `json:"scan"`
}

// List answers the sketch's schedules with their last run summed up.
func (h Handler) List(c fiber.Ctx) error {
	sketchID := access.Sketch(c).ID
	all, err := h.Runtime.Store.Schedule.ListBySketch(c, sketchID)
	if err != nil {
		return response.Internal(c, err)
	}

	out := listResponse{Schedules: []listed{}}
	var nodes []graphmodel.Node
	for _, s := range all {
		l := listed{Schedule: s}
		if s.LastScanID != "" {
			if nodes == nil {
				if nodes, _, err = h.Runtime.Store.Graph.Graph(c, sketchID); err != nil {
					return response.Internal(c, err)
				}
			}

			sum, err := h.summary(c, s, nodes)
			if err != nil {
				return response.Internal(c, err)
			}

			l.Summary = sum
		}

		out.Schedules = append(out.Schedules, l)
	}

	return response.Success(c, out)
}

// summary counts the last run's findings off the nodes' origin and
// seen marks.
func (h Handler) summary(ctx context.Context, s smodel.Schedule, nodes []graphmodel.Node) (*Summary, error) {
	sc, err := h.Runtime.Store.Scan.Get(ctx, s.LastScanID)
	if err != nil {
		return nil, err
	}

	sum := &Summary{ScanID: s.LastScanID}
	if sc != nil {
		sum.Status = string(sc.Status)
	}

	scanOf := func(m map[string]any, key string) string {
		o, _ := m[key].(map[string]any)
		id, _ := o["scan"].(string)

		return id
	}

	for _, n := range nodes {
		origin, seen := scanOf(n.Metadata, graphmodel.OriginKey), scanOf(n.Metadata, graphmodel.SeenKey)
		switch {
		case origin == s.LastScanID:
			sum.New++
		case seen == s.LastScanID:
			sum.Seen++
		case s.PrevScanID != "" && seen == s.PrevScanID:
			sum.Missing++
		}
	}

	return sum, nil
}

// Create adds a schedule for one of the caller's flows.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	user := access.User(c).ID
	f, err := h.Runtime.Store.Flow.Get(c, user, in.FlowID)
	if err != nil {
		return response.Internal(c, err)
	}

	if f == nil {
		return response.BadRequestFields(c, "flow not found",
			[]validation.FieldError{{Field: "flow_id", Rule: "exists", Message: "no such flow of yours"}})
	}

	now := time.Now().UTC()
	s := &smodel.Schedule{ID: ids.New(), SketchID: access.Sketch(c).ID, OwnerID: user, FlowID: f.ID, FlowName: f.Name,
		NodeIDs: in.NodeIDs, Every: in.Every, Enabled: in.Enabled == nil || *in.Enabled, CreatedAt: now, UpdatedAt: now}
	if s.NodeIDs == nil {
		s.NodeIDs = []string{}
	}

	if err := h.Runtime.Store.Schedule.Create(c, s); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, oneResponse{Schedule: listed{Schedule: *s}})
}

// Update changes the interval, the selection or the switch.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[saveInput](c)
	if !ok {
		return resp
	}

	s, resp, ok := h.own(c)
	if !ok {
		return resp
	}

	s.Every = in.Every
	if in.NodeIDs != nil {
		s.NodeIDs = in.NodeIDs
	}

	if in.Enabled != nil {
		s.Enabled = *in.Enabled
	}

	s.UpdatedAt = time.Now().UTC()
	if err := h.Runtime.Store.Schedule.Update(c, s); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, oneResponse{Schedule: listed{Schedule: *s}})
}

// Delete removes a schedule.
func (h Handler) Delete(c fiber.Ctx) error {
	s, resp, ok := h.own(c)
	if !ok {
		return resp
	}

	ok, err := h.Runtime.Store.Schedule.Delete(c, access.Sketch(c).ID, s.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if !ok {
		return response.NotFound(c, "schedule not found")
	}

	return response.NoContent(c)
}

// Run queues the schedule's flow now, as its owner.
func (h Handler) Run(c fiber.Ctx) error {
	s, resp, ok := h.own(c)
	if !ok {
		return resp
	}

	sc, err := Run(c, h.Runtime, s, time.Now().UTC())
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	if sc == nil {
		return response.Conflict(c, "another node ran it just now")
	}

	return response.Created(c, runResponse{Scan: *sc})
}

// own loads the schedule in the path for a change. A schedule runs as
// its owner with the owner's secrets, so only the owner or a member
// who manages the investigation may change, run or remove it.
func (h Handler) own(c fiber.Ctx) (*smodel.Schedule, error, bool) {
	s, err := h.Runtime.Store.Schedule.Get(c, access.Sketch(c).ID, c.Params("schedule_id"))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if s == nil {
		return nil, response.NotFound(c, "schedule not found"), false
	}

	if s.OwnerID != access.User(c).ID && !access.Investigation(c).Role.Can(invmodel.ActionManage) {
		return nil, response.Forbidden(c, "only the person who scheduled it or a manager of the investigation can change it"), false
	}

	return s, nil, true
}

// Run queues one run of s as its owner: the flow is read again (its
// steps may have changed), the inputs are the selection or every
// entity of the input type, and the schedule is claimed before the
// scan is queued, so two nodes never queue it twice. Nil when another
// node claimed it first. A flow that is gone switches the schedule off.
func Run(ctx context.Context, rt *env.Runtime, s *smodel.Schedule, now time.Time) (*scanmodel.Scan, error) {
	if reason, err := ownerMayRun(ctx, rt, s); err != nil {
		return nil, err
	} else if reason != "" {
		_ = rt.Store.Schedule.Fail(ctx, s.ID, reason, now)

		return nil, fmt.Errorf("%s, the schedule is off", reason)
	}

	f, err := rt.Store.Flow.Get(ctx, s.OwnerID, s.FlowID)
	if err != nil {
		return nil, err
	}

	if f == nil {
		_ = rt.Store.Schedule.Fail(ctx, s.ID, "the flow is gone", now)

		return nil, fmt.Errorf("the flow %s is gone, the schedule is off", s.FlowName)
	}

	if err := coreflow.Check(f.Definition, rt.Transforms); err != nil {
		_ = rt.Store.Schedule.Fail(ctx, s.ID, err.Error(), now)

		return nil, fmt.Errorf("the flow does not run here: %w", err)
	}

	inputs := s.NodeIDs
	if len(inputs) == 0 {
		nodes, _, err := rt.Store.Graph.Graph(ctx, s.SketchID)
		if err != nil {
			return nil, err
		}

		for _, n := range nodes {
			if n.Type == f.Definition.Input && len(inputs) < maxInputs {
				inputs = append(inputs, n.ID)
			}
		}
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("the sketch holds no %s to run on", f.Definition.Input)
	}

	usable, err := scan.UsableNodes(ctx, rt, s.SketchID, inputs, f.Definition.Input)
	if err != nil {
		return nil, err
	}

	if usable == 0 {
		return nil, fmt.Errorf("none of the selected entities is a %s any more", f.Definition.Input)
	}

	nodeIDs := make([]any, 0, len(inputs))
	for _, id := range inputs {
		nodeIDs = append(nodeIDs, id)
	}

	sc := &scanmodel.Scan{ID: ids.New(), SketchID: s.SketchID, OwnerID: s.OwnerID, Kind: coreflow.ScanKind,
		Name: f.Name + " (scheduled)", Input: map[string]any{"flow_id": f.ID, "node_ids": nodeIDs, "schedule_id": s.ID}}

	claimed, err := rt.Store.Schedule.Claim(ctx, s, sc.ID, now)
	if err != nil || !claimed {
		return nil, err
	}

	if err := scan.Enqueue(ctx, rt, sc); err != nil {
		return nil, err
	}

	return sc, nil
}

// ownerMayRun says why the schedule's owner may no longer run it on its
// sketch, "" when they may: the run spends their secrets and acts on the
// sketch as them, so it stops once the account is disabled or loses the
// right to edit the investigation.
func ownerMayRun(ctx context.Context, rt *env.Runtime, s *smodel.Schedule) (string, error) {
	owner, err := rt.Store.User.Get(ctx, s.OwnerID)
	if err != nil {
		return "", err
	}

	if owner == nil || !owner.Active {
		return "its owner's account is disabled", nil
	}

	sk, err := rt.Store.Sketch.Get(ctx, s.SketchID)
	if err != nil || sk == nil {
		return "", err
	}

	role, err := rt.Store.Investigation.Role(ctx, sk.InvestigationID, s.OwnerID)
	if err != nil {
		return "", err
	}

	if !role.Can(invmodel.ActionUpdate) {
		return "its owner may no longer edit this investigation", nil
	}

	return "", nil
}

// Tick runs every due schedule, the maintenance job's body. One failing
// to queue is logged and switched off by Run, the rest go on.
func Tick(ctx context.Context, rt *env.Runtime) error {
	now := time.Now().UTC()
	all, err := rt.Store.Schedule.ListEnabled(ctx)
	if err != nil {
		return err
	}

	for i := range all {
		s := &all[i]
		if !s.Due(now) {
			continue
		}

		if _, err := Run(ctx, rt, s, now); err != nil {
			rt.Log.Warn("schedule: run not queued", "schedule", s.ID, "flow", s.FlowName, "err", err)
		}
	}

	return nil
}
