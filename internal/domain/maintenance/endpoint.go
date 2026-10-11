// Gosint, Copyright (c) 2026 YouSysAdmin

// Package maintenance shows an administrator the scheduled jobs and
// runs one on request.
package maintenance

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
)

// Handler serves the jobs to the admin panel.
type Handler struct {
	Runtime *env.Runtime
}

// jobsResponse lists the jobs of this node.
type jobsResponse struct {
	Jobs []cron.Status `json:"jobs"`
}

// List answers every job with its last and next run.
func (h Handler) List(c fiber.Ctx) error {
	return response.Success(c, jobsResponse{Jobs: h.Runtime.Cron.Statuses()})
}

// Run runs one job now, on this node, and answers once it is done.
func (h Handler) Run(c fiber.Ctx) error {
	err := h.Runtime.Cron.RunNow(c, c.Params("name"))
	switch {
	case errors.Is(err, cron.ErrUnknownJob):
		return response.NotFound(c, "no such job")
	case errors.Is(err, cron.ErrJobRunning):
		return response.Conflict(c, "the job is running already")
	case err != nil:
		// The raw error may name paths or a bucket, the cron has logged
		// it and the job list carries it to the administrator.
		return response.Coded(c, fiber.StatusBadGateway, "the job failed, its error is in the job list")
	}

	return response.Success(c, jobsResponse{Jobs: h.Runtime.Cron.Statuses()})
}
