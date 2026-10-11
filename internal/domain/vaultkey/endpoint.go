// Gosint, Copyright (c) 2026 YouSysAdmin

package vaultkey

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	keymodel "github.com/yousysadmin/gosint/internal/models/vaultkey"
)

// Handler serves the caller's own keys.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the caller's key names.
func (h Handler) List(c fiber.Ctx) error {
	keys, err := h.Runtime.Store.VaultKey.List(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, keysResponse{Keys: keys})
}

// Put seals and stores a key.
func (h Handler) Put(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[putInput](c)
	if !ok {
		return resp
	}

	owner := access.User(c).ID
	aad, err := ValueAAD(h.Runtime.Vault, owner, in.Name)
	if err != nil {
		return response.Internal(c, err)
	}

	sealed, err := h.Runtime.Vault.SealText(aad, in.Value)
	if err != nil {
		return response.Internal(c, err)
	}

	now := time.Now().UTC()
	k := &keymodel.Key{ID: ids.New(), OwnerID: owner, Name: in.Name, Stored: sealed, CreatedAt: now, UpdatedAt: now}
	if err := h.Runtime.Store.VaultKey.Put(c, k); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, keyResponse{Key: *k})
}

// Delete removes one of the caller's keys.
func (h Handler) Delete(c fiber.Ctx) error {
	found, err := h.Runtime.Store.VaultKey.Delete(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !found {
		return response.NotFound(c, "key not found")
	}

	return response.NoContent(c)
}

// Secret opens the owner's key named ref, or with ref as its id, for an
// transform. False when there is none.
func Secret(ctx context.Context, rt *env.Runtime, ownerID, ref string) (string, bool, error) {
	k, err := rt.Store.VaultKey.GetByName(ctx, ownerID, ref)
	if err == nil && k == nil && ids.Valid(ref) {
		k, err = rt.Store.VaultKey.Get(ctx, ownerID, ref)
	}

	if err != nil || k == nil {
		return "", false, err
	}

	aad, err := ValueAAD(rt.Vault, ownerID, k.Name)
	if err != nil {
		return "", false, err
	}

	plain, err := rt.Vault.OpenText(aad, k.Stored)
	if err != nil {
		return "", false, err
	}

	return plain, true, nil
}
