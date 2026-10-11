// Gosint, Copyright (c) 2026 YouSysAdmin

// Package access carries what the auth and access middleware resolved
// for a request, so handlers read it instead of resolving it again.
//
// A handler behind requireSession may call User, behind
// requireInvestigation may call Investigation, behind requireSketch
// may call Sketch. Each panics when its middleware did not run, which
// is a routing bug and not a request to answer.
package access

import (
	"github.com/gofiber/fiber/v3"

	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sessionmodel "github.com/yousysadmin/gosint/internal/models/session"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

type key int

const (
	userKey key = iota
	sessionKey
	investigationKey
	sketchKey
)

// Scope is the investigation a request acts on and the caller's role.
type Scope struct {
	Investigation *invmodel.Investigation
	Role          invmodel.Role
}

// SetUser records the signed-in account.
func SetUser(c fiber.Ctx, u *usermodel.User) { c.Locals(userKey, u) }

// User is the signed-in account.
func User(c fiber.Ctx) *usermodel.User { return mustLocal[*usermodel.User](c, userKey) }

// MaybeUser is the signed-in account, or nil when the request carried
// no session. For a public route behind optionalSession.
func MaybeUser(c fiber.Ctx) *usermodel.User {
	u, _ := c.Locals(userKey).(*usermodel.User)

	return u
}

// SetSession records the session the request rides on.
func SetSession(c fiber.Ctx, s *sessionmodel.Session) { c.Locals(sessionKey, s) }

// Session is the session the request rides on, so a handler can tell
// it from the account's other ones.
func Session(c fiber.Ctx) *sessionmodel.Session {
	return mustLocal[*sessionmodel.Session](c, sessionKey)
}

// SetInvestigation records the investigation and role.
func SetInvestigation(c fiber.Ctx, s Scope) { c.Locals(investigationKey, s) }

// Investigation is the investigation and the caller's role on it.
func Investigation(c fiber.Ctx) Scope { return mustLocal[Scope](c, investigationKey) }

// SetSketch records the sketch, whose investigation is set alongside.
func SetSketch(c fiber.Ctx, s *sketchmodel.Sketch) { c.Locals(sketchKey, s) }

// Sketch is the sketch the request acts on.
func Sketch(c fiber.Ctx) *sketchmodel.Sketch { return mustLocal[*sketchmodel.Sketch](c, sketchKey) }

func mustLocal[T any](c fiber.Ctx, k key) T {
	v, ok := c.Locals(k).(T)
	if !ok {
		panic("access: middleware for this value did not run")
	}

	return v
}
