// Gosint, Copyright (c) 2026 YouSysAdmin

// Package response wraps Fiber JSON responses behind named helpers so
// status codes and the error envelope stay consistent across the API.
//
// Errors are {"error": "msg"}, plus "fields" for a validation refusal.
package response

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/database"
)

// errorBody is the error envelope.
type errorBody struct {
	Error string `json:"error"`
}

// Success writes 200 with data as the body.
func Success(c fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(data)
}

// Created writes 201 with data as the body.
func Created(c fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(data)
}

// NoContent writes 204 and no body.
func NoContent(c fiber.Ctx) error {
	return c.SendStatus(fiber.StatusNoContent)
}

// BadRequest writes 400 with msg.
func BadRequest(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(errorBody{msg})
}

// fieldsBody is the error envelope naming the refused fields.
type fieldsBody struct {
	Error  string `json:"error"`
	Fields any    `json:"fields"`
}

// BadRequestFields writes 400 with a summary and the refused fields,
// so the console can place each message under its input.
func BadRequestFields(c fiber.Ctx, summary string, fields any) error {
	return c.Status(fiber.StatusBadRequest).JSON(fieldsBody{Error: summary, Fields: fields})
}

// Unauthorized writes 401, meaning the caller is not identified.
func Unauthorized(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(errorBody{msg})
}

// Forbidden writes 403, meaning the caller is identified and still may not.
func Forbidden(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusForbidden).JSON(errorBody{msg})
}

// NotFound writes 404. Access to another user's investigation answers
// this too, so a foreign row cannot be told from a missing one.
func NotFound(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusNotFound).JSON(errorBody{msg})
}

// Conflict writes 409 for a state the caller can fix.
func Conflict(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusConflict).JSON(errorBody{msg})
}

// Gone answers 410, for a link that was valid once.
func Gone(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusGone).JSON(errorBody{msg})
}

// TooManyRequests writes 429, the caller may retry later.
func TooManyRequests(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusTooManyRequests).JSON(errorBody{msg})
}

// Unavailable writes 503 for a temporary refusal.
func Unavailable(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(errorBody{msg})
}

// Coded answers with an explicit status, for a status no helper above
// names (the app-wide error handler repeats the one Fiber chose).
func Coded(c fiber.Ctx, code int, msg string) error {
	return c.Status(code).JSON(errorBody{msg})
}

// Internal logs err and answers a generic 500. The raw error is never
// echoed back.
//
// A malformed id is a 404 and a unique key violation is a 409, decided
// here so every store error is translated in one place.
func Internal(c fiber.Ctx, err error) error {
	switch {
	case database.MalformedID(err):
		slog.Warn("request carried an id that is not a uuid", "err", err, "path", c.Path(), "method", c.Method())

		return NotFound(c, "not found")
	case database.UniqueViolation(err):
		slog.Warn("write refused by a unique key", "err", err, "path", c.Path(), "method", c.Method())

		return Conflict(c, "a resource with the same unique value already exists")
	case err != nil:
		slog.Error("handler internal error", "err", err, "path", c.Path(), "method", c.Method())
	}

	return c.Status(fiber.StatusInternalServerError).JSON(errorBody{"internal server error"})
}
