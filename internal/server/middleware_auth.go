// Gosint, Copyright (c) 2026 YouSysAdmin

package server

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// requireSession resolves the session cookie to an active account, or
// answers 401.
func requireSession(rt *env.Runtime) fiber.Handler {
	return func(c fiber.Ctx) error {
		token := c.Cookies(auth.CookieName)
		u, sess, renewed, err := auth.Resolve(c, rt, token)
		if err != nil {
			return response.Internal(c, err)
		}

		if u == nil {
			return response.Unauthorized(c, "authentication required")
		}

		if renewed {
			auth.RenewCookie(c, rt, token, sess)
		}

		access.SetUser(c, u)
		access.SetSession(c, sess)

		return c.Next()
	}
}

// optionalSession resolves the session cookie when there is one and
// lets the request through either way, for a public route that reads
// differently to a signed-in person.
func optionalSession(rt *env.Runtime) fiber.Handler {
	return func(c fiber.Ctx) error {
		u, sess, _, err := auth.Resolve(c, rt, c.Cookies(auth.CookieName))
		if err != nil {
			return response.Internal(c, err)
		}

		if u != nil {
			access.SetUser(c, u)
			access.SetSession(c, sess)
		}

		return c.Next()
	}
}

// requireUnsealed answers 503 to every API request while the vault holds
// no key, except the seal routes that give it one.
func requireUnsealed(rt *env.Runtime) fiber.Handler {
	return func(c fiber.Ctx) error {
		if !rt.Vault.IsSealed() || strings.HasPrefix(c.Path(), "/api/sys/") {
			return c.Next()
		}

		return response.Unavailable(c, "sealed")
	}
}

// requireAdmin refuses a caller who does not administer the install.
// Always after requireSession.
func requireAdmin(c fiber.Ctx) error {
	if !access.User(c).Admin {
		return response.Forbidden(c, "administrator privileges required")
	}

	return c.Next()
}

// refuseCrossSite refuses a mutating request from an origin that is not
// ours, since the session cookie rides on it.
//
// SameSite=Lax keeps the cookie off a cross-site POST, but a sibling
// host on the same site still gets it. The check is the browser's own
// word: Origin on every cross-origin POST, Sec-Fetch-Site on every
// request, neither settable by a page. A client sending neither is not
// a browser and has no ambient cookie to forge with.
func refuseCrossSite(rt *env.Runtime) fiber.Handler {
	public := originOf(rt.Config.Server.PublicURL)

	return func(c fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return c.Next()
		}

		origin := strings.ToLower(c.Get(fiber.HeaderOrigin))
		if origin == "" {
			switch c.Get("Sec-Fetch-Site") {
			case "cross-site", "same-site":
				return response.Forbidden(c, "cross-site request refused")
			}

			return c.Next()
		}

		if origin == public || origin == c.Scheme()+"://"+strings.ToLower(c.Host()) {
			return c.Next()
		}

		return response.Forbidden(c, "cross-site request refused")
	}
}

// originOf reduces a URL to scheme://host[:port], lowercased, or "".
func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}

	return strings.ToLower(u.Scheme + "://" + u.Host)
}
