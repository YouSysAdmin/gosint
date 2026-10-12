// Gosint, Copyright (c) 2026 YouSysAdmin

// Package console serves the settings the browser console reads.
package console

import (
	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/release"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/pkg"
)

// Handler serves the console settings.
type Handler struct {
	Runtime *env.Runtime
}

// Settings answers the operator's console configuration.
func (h Handler) Settings(c fiber.Ctx) error {
	cfg := h.Runtime.Config.Console

	return response.Success(c, settingsResponse{MapStyleLight: cfg.MapStyleLight, MapStyleDark: cfg.MapStyleDark})
}

// Release answers whether a newer stable release is out, for the
// administrator's badge. With the check off it names the running
// version only.
func (h Handler) Release(c fiber.Ctx) error {
	if h.Runtime.Release == nil {
		return response.Success(c, release.Status{Current: pkg.Version})
	}

	return response.Success(c, h.Runtime.Release.Status(c))
}
