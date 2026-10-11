// Gosint, Copyright (c) 2026 YouSysAdmin

package sys

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/envelope"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/core/vault"
)

// refusalFloor is the least time a wrong unseal key takes to answer,
// on top of Argon2id and the per-address limiter.
const refusalFloor = 750 * time.Millisecond

// Handler serves the seal routes, the only ones a sealed server answers.
type Handler struct {
	Runtime *env.Runtime
}

func (h Handler) store() *Store { return NewStore(h.Runtime.DB) }

// Status answers whether the server is initialized and sealed.
func (h Handler) Status(c fiber.Ctx) error {
	row, err := h.store().Get(c)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, statusResponse{
		Initialized:  row != nil,
		Sealed:       h.Runtime.Vault.IsSealed(),
		MinUnsealKey: MinUnsealKey,
	})
}

// Init wraps the master key under the first unseal key, once.
func (h Handler) Init(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[unsealKeyInput](c)
	if !ok {
		return resp
	}

	// An open vault is initialized from the CLI only: over HTTP anyone
	// could wrap the running key under a key of their own.
	if !h.Runtime.Vault.IsSealed() {
		return response.Conflict(c, "the server is open, initialize it with `gosint vault init`")
	}

	err := Initialize(c, h.store(), h.Runtime.Vault, in.UnsealKey)
	switch {
	case errors.Is(err, envelope.ErrBusy):
		return response.Unavailable(c, err.Error())
	case errors.Is(err, ErrShortUnsealKey):
		return response.BadRequestFields(c, err.Error(), []validation.FieldError{{Field: "unseal_key", Rule: "min", Message: err.Error()}})
	case errors.Is(err, ErrInitialized), errors.Is(err, ErrKeyLost):
		return response.Conflict(c, err.Error())
	case err != nil:
		return response.Internal(c, err)
	}

	h.Runtime.Log.Info("vault initialized and unsealed")

	return h.Status(c)
}

// Unseal unwraps the master key with the unseal key.
func (h Handler) Unseal(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[unsealKeyInput](c)
	if !ok {
		return resp
	}

	// An open vault answers without deriving a key: a guess against a
	// running server costs it nothing and tells nothing.
	if !h.Runtime.Vault.IsSealed() {
		return h.Status(c)
	}

	start := time.Now()
	err := Unseal(c, h.store(), h.Runtime.Vault, in.UnsealKey)
	switch {
	case errors.Is(err, envelope.ErrBusy):
		return response.Unavailable(c, err.Error())
	case errors.Is(err, vault.ErrUnsealKey):
		if wait := refusalFloor - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}

		h.Runtime.Log.Warn("vault: a wrong unseal key", "ip", c.IP())

		return response.BadRequestFields(c, "the unseal key does not open this server",
			[]validation.FieldError{{Field: "unseal_key", Rule: "unseal_key", Message: "wrong unseal key"}})
	case errors.Is(err, ErrNotInitialized):
		return response.Conflict(c, err.Error())
	case err != nil:
		return response.Internal(c, err)
	}

	h.Runtime.Log.Info("vault unsealed")

	return h.Status(c)
}

// Seal forgets the master key on this node until the next unseal.
func (h Handler) Seal(c fiber.Ctx) error {
	h.Runtime.Vault.Reseal()
	h.Runtime.Log.Warn("vault sealed by an administrator")

	return h.Status(c)
}
