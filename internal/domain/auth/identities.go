// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/response"
)

// Identities lists the providers linked to the caller's account, each
// with the provider's name, so the profile can offer Unlink.
func (h Handler) Identities(c fiber.Ctx) error {
	u := access.User(c)
	idents, err := h.Runtime.Store.OAuthIdentity.ListForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	providers, err := h.Runtime.Store.OAuthProvider.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	names := make(map[string][2]string, len(providers))
	for _, p := range providers {
		names[p.ID] = [2]string{p.Name, p.Slug}
	}

	out := identitiesResponse{Identities: make([]identityView, 0, len(idents))}
	for _, i := range idents {
		n := names[i.ProviderID]
		out.Identities = append(out.Identities, identityView{ID: i.ID, ProviderName: n[0], ProviderSlug: n[1],
			Email: i.Email, CreatedAt: i.CreatedAt, LastLoginAt: i.LastLoginAt})
	}

	return response.Success(c, out)
}

// Unlink removes one of the caller's identities, unless it is the last
// way into the account: no password, no passkey, no other provider.
func (h Handler) Unlink(c fiber.Ctx) error {
	u := access.User(c)
	id := c.Params("id")
	idents, err := h.Runtime.Store.OAuthIdentity.ListForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	mine := false
	for _, i := range idents {
		mine = mine || i.ID == id
	}

	if !mine {
		return response.NotFound(c, "not found")
	}

	if len(idents) == 1 && (!u.Local() || u.PasswordHash == "") {
		keys, err := h.Runtime.Store.Passkey.CountForUser(c, u.ID)
		if err != nil {
			return response.Internal(c, err)
		}

		if keys == 0 {
			return response.Conflict(c, "this provider is the only way into your account, add a password or a passkey first")
		}
	}

	if _, err := h.Runtime.Store.OAuthIdentity.DeleteForUser(c, id, u.ID); err != nil {
		return response.Internal(c, err)
	}

	return response.NoContent(c)
}
