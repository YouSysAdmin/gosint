// Gosint, Copyright (c) 2026 YouSysAdmin

// Package console serves the settings the browser console reads.
package console

import (
	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
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
