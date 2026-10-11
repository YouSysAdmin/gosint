// Gosint, Copyright (c) 2026 YouSysAdmin

package setting

import (
	"cmp"
	"context"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/core/worker"
)

// workerValue is the worker's tuning as stored and on the wire, the
// durations in seconds.
type workerValue struct {
	// Enabled lets the workers claim scans, off they wait queued.
	Enabled bool `json:"enabled"`

	// CacheDir is where transforms keep downloaded reference data, an
	// absolute path, empty for the default beside the database.
	CacheDir string `json:"cache_dir" validate:"max=1024" normalize:"trim"`

	Concurrency  int `json:"concurrency"   validate:"min=1,max=64"`
	PollSeconds  int `json:"poll_seconds"  validate:"min=1,max=300"`
	LeaseSeconds int `json:"lease_seconds" validate:"min=10,max=3600"`
	MaxAttempts  int `json:"max_attempts"  validate:"min=1,max=20"`
}

func workerValueOf(t worker.Tuning) workerValue {
	return workerValue{Enabled: t.Enabled, Concurrency: t.Concurrency, PollSeconds: int(t.Poll / time.Second),
		LeaseSeconds: int(t.Lease / time.Second), MaxAttempts: t.MaxAttempts}
}

func (v workerValue) tuning() worker.Tuning {
	return worker.Tuning{Enabled: v.Enabled, Concurrency: v.Concurrency, Poll: time.Duration(v.PollSeconds) * time.Second,
		Lease: time.Duration(v.LeaseSeconds) * time.Second, MaxAttempts: v.MaxAttempts}
}

// readWorker is the stored tuning, the default when none was saved.
func readWorker(ctx context.Context, rt *env.Runtime) (workerValue, error) {
	v := workerValueOf(worker.DefaultTuning())
	_, err := rt.Store.Setting.Get(ctx, Worker, &v)

	return v, err
}

// apply hands the worker settings to the running node.
func (h Handler) apply(v workerValue) { applyWorker(h.Runtime, v) }

func applyWorker(rt *env.Runtime, v workerValue) {
	rt.WorkerTuning.Set(v.tuning())
	rt.CacheDir.Set(cmp.Or(v.CacheDir, rt.Config.DefaultCacheDir()))
}

// GetWorker answers the worker's tuning.
func (h Handler) GetWorker(c fiber.Ctx) error {
	v, err := readWorker(c, h.Runtime)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, workerResponse{Worker: v, DefaultCacheDir: h.Runtime.Config.DefaultCacheDir()})
}

// PutWorker saves the worker settings. This node's worker applies it on its next
// pass, the others once the refresh job has read it.
func (h Handler) PutWorker(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[workerValue](c)
	if !ok {
		return resp
	}

	if in.CacheDir != "" && !filepath.IsAbs(in.CacheDir) {
		return refuse(c, "cache_dir", "abs", "the cache directory must be an absolute path")
	}

	if err := h.Runtime.Store.Setting.Put(c, Worker, in, access.User(c).ID); err != nil {
		return response.Internal(c, err)
	}

	h.apply(in)

	return response.Success(c, workerResponse{Worker: in, DefaultCacheDir: h.Runtime.Config.DefaultCacheDir()})
}
