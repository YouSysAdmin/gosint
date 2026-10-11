// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	blobfs "github.com/yousysadmin/gosint/internal/blob/fs"
	"github.com/yousysadmin/gosint/internal/blob/sealed"

	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/live"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/oidc"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/transforms"
)

// testRuntime is a runtime over db with a throwaway secret and no
// worker.
func testRuntime(t *testing.T, db database.Database, cfg *env.Config) *env.Runtime {
	t.Helper()

	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	bus := eventbus.New()
	t.Cleanup(bus.Close)

	files, err := blobfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	blobs := sealed.New(files, v)

	return &env.Runtime{
		Config:     cfg,
		Log:        slog.New(slog.DiscardHandler),
		DB:         db,
		Store:      backend.BindStore(db, v),
		Vault:      v,
		Events:     bus,
		Transforms: transform.NewRegistry(transforms.All()...),

		Tools:        tool.NewRegistry(transforms.Tools()...),
		ToolSettings: live.New(tool.Settings{}),
		WorkerTuning: live.New(worker.DefaultTuning()),
		CacheDir:     live.New(t.TempDir()),
		TargetHTTP:   http.DefaultClient,
		WakeWorker:   func() {},
		Blobs:        blobs,
		FileLimits:   live.New(env.FileLimits{MaxSize: 4 << 20, RetainDeleted: 24 * time.Hour}),
		Mail:         &mailer.Fake{},
		OAuth:        oidc.NewRegistry(cfg.Server.PublicURL),
		Cron:         cron.New(slog.New(slog.DiscardHandler)),
	}
}
