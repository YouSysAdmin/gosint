// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/vault"
)

// A short-lived state the browser carries between two requests, the
// second factor step and a passkey ceremony, rides in a cookie sealed
// by the vault under the cookie's name. The vault has no expiry, so the
// payload carries its own, checked on open. HttpOnly and SameSite
// Strict: nothing but this site's own next request reads it.

// envelope is what a sealed cookie holds.
type envelope struct {
	Expires time.Time `json:"exp"`
	Payload []byte    `json:"p"`
}

// errNoState is a cookie that is missing, expired or not ours.
var errNoState = errors.New("no state in progress")

// sealCookie stores v in the cookie named name for ttl.
func sealCookie(c fiber.Ctx, rt *env.Runtime, name string, v any, ttl time.Duration) error {
	payload, err := json.Marshal(v)
	if err != nil {
		return err
	}

	expires := time.Now().UTC().Add(ttl)
	raw, err := json.Marshal(envelope{Expires: expires, Payload: payload})
	if err != nil {
		return err
	}

	sealed, err := rt.Vault.Seal("cookie:"+name, string(raw))
	if err != nil {
		return err
	}

	c.Cookie(&fiber.Cookie{
		Name: name, Value: sealed.Encode(), Path: "/", Expires: expires,
		HTTPOnly: true, Secure: secureCookie(c, rt), SameSite: fiber.CookieSameSiteStrictMode,
	})

	return nil
}

// openCookie reads v back from the cookie named name. The cookie stays,
// a caller that spends it clears it with clearNamed.
func openCookie[T any](c fiber.Ctx, rt *env.Runtime, name string) (*T, error) {
	raw := c.Cookies(name)
	if raw == "" {
		return nil, errNoState
	}

	sealed, err := vault.DecodeSealed(raw)
	if err != nil {
		return nil, errNoState
	}

	plain, err := rt.Vault.Open("cookie:"+name, sealed)
	if err != nil {
		return nil, errNoState
	}

	var env envelope
	if err := json.Unmarshal([]byte(plain), &env); err != nil || time.Now().After(env.Expires) {
		return nil, errNoState
	}

	var v T
	if err := json.Unmarshal(env.Payload, &v); err != nil {
		return nil, errNoState
	}

	return &v, nil
}

// clearNamed removes a sealed cookie from the browser.
func clearNamed(c fiber.Ctx, rt *env.Runtime, name string) {
	c.Cookie(&fiber.Cookie{
		Name: name, Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1,
		HTTPOnly: true, Secure: secureCookie(c, rt), SameSite: fiber.CookieSameSiteStrictMode,
	})
}
