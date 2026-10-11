// Gosint, Copyright (c) 2026 YouSysAdmin

package server

import (
	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
)

// requireInvestigation loads the investigation of the :id param and
// refuses a caller whose role does not allow action.
//
// A non-member gets 404, the same as a missing investigation, so ids of
// other people's work cannot be probed. A member lacking the action
// gets 403.
func requireInvestigation(rt *env.Runtime, action invmodel.Action) fiber.Handler {
	return func(c fiber.Ctx) error {
		ok, err := authorize(c, rt, c.Params("id"), action)
		if !ok {
			return err
		}

		return c.Next()
	}
}

// requireSketch loads the sketch of the :id param and checks action on
// its investigation, with the same 404 and 403 as requireInvestigation.
// A missing sketch reads exactly like one in somebody else's
// investigation, so ids cannot be probed.
func requireSketch(rt *env.Runtime, action invmodel.Action) fiber.Handler {
	return func(c fiber.Ctx) error {
		sk, err := rt.Store.Sketch.Get(c, c.Params("id"))
		if err != nil {
			return response.Internal(c, err)
		}

		if sk == nil {
			return response.NotFound(c, "not found")
		}

		ok, err := authorize(c, rt, sk.InvestigationID, action)
		if !ok {
			return err
		}

		access.SetSketch(c, sk)

		return c.Next()
	}
}

// authorize resolves the caller's role on investigationID. When ok is
// false the refusal has been written and err is what to return.
func authorize(c fiber.Ctx, rt *env.Runtime, investigationID string, action invmodel.Action) (bool, error) {
	role, err := rt.Store.Investigation.Role(c, investigationID, access.User(c).ID)
	if err != nil {
		return false, response.Internal(c, err)
	}

	if role == "" {
		return false, response.NotFound(c, "not found")
	}

	if !role.Can(action) {
		return false, response.Forbidden(c, "your role on this investigation does not allow this")
	}

	inv, err := rt.Store.Investigation.Get(c, investigationID)
	if err != nil {
		return false, response.Internal(c, err)
	}

	if inv == nil {
		return false, response.NotFound(c, "not found")
	}

	access.SetInvestigation(c, access.Scope{Investigation: inv, Role: role})

	return true, nil
}
