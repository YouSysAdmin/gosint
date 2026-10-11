// Gosint, Copyright (c) 2026 YouSysAdmin

// Package paging reads list bounds from the query string.
package paging

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

// Limit reads ?limit=, falling back to def and capped at max. A value
// that is not a positive number is the default, not an error.
func Limit(c fiber.Ctx, def, maximum int) int {
	n, err := strconv.Atoi(c.Query("limit"))
	if err != nil || n <= 0 {
		return def
	}

	return min(n, maximum)
}
