// Gosint, Copyright (c) 2026 YouSysAdmin

package env

import (
	"log/slog"
	"net/http"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/live"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/oidc"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/store"
)

// Runtime is the server-scoped bag of dependencies. Built once in
// cli/serve.go and handed to every domain Handler.
type Runtime struct {
	Config *Config
	Log    *slog.Logger
	DB     database.Database
	Store  *store.Store

	// Vault seals and opens what is stored. Always set, sealed until
	// the operator's unseal key unwraps the master key.
	Vault *vault.Vault

	// Events fans sketch activity out to live streams. Always set.
	Events *eventbus.Bus

	// Transforms are the transforms this build runs. Always set.
	Transforms *transform.Registry

	// Tools are the tools the transforms use, ToolSettings what an
	// administrator set for them. Always set.
	Tools        *tool.Registry
	ToolSettings *live.Value[tool.Settings]

	// WorkerTuning is the worker's tuning an administrator set, read on
	// every pass of the worker. Always set.
	WorkerTuning *live.Value[worker.Tuning]

	// CacheDir is where transforms keep downloaded reference data: the
	// worker settings' directory, else Config.DefaultCacheDir. Always set.
	CacheDir *live.Value[string]

	// TargetHTTP fetches what an entity or a template names, refusing
	// internal addresses (transform.TargetHTTPClient). Always set.
	TargetHTTP *http.Client

	// WakeWorker tells this process's worker a scan was queued. A no-op
	// on a node without a worker, whose scans other nodes poll for.
	WakeWorker func()

	// Blobs holds the bytes of files attached to entities. Always set,
	// pointed at the file store the settings name once unsealed.
	Blobs blob.Store

	// FileLimits bound an upload and how long deleted bytes stay.
	// Always set.
	FileLimits *live.Value[FileLimits]

	// Mail sends the server's own messages, reset and invitation links.
	// Always set, disabled when no relay is configured, so every caller
	// checks Enabled and degrades.
	Mail mailer.Sender

	// OAuth builds and caches the identity providers' flows. Always set.
	OAuth *oidc.Registry

	// Cron runs the maintenance jobs. Always set, every node runs every
	// job, so a job is a convergent sweep.
	Cron *cron.Manager
}

// StateKey signs the OIDC round-trip cookie, derived from the vault
// master key so every node agrees.
func (rt *Runtime) StateKey() ([]byte, error) {
	return rt.Vault.Derive("gosint/oidc-state")
}
