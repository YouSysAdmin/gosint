// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"context"
	"time"

	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/customtype"
	"github.com/yousysadmin/gosint/internal/domain/schedule"
	"github.com/yousysadmin/gosint/internal/domain/setting"
)

// registerJobs puts the maintenance sweeps on rt.Cron. Every node runs
// every job, each a delete by age that two nodes may run together.
func registerJobs(rt *env.Runtime) {
	register := func(j cron.Job) {
		run := j.Run
		j.Run = func(ctx context.Context) error {
			if rt.Vault.IsSealed() {
				return nil
			}

			return run(ctx)
		}

		rt.Cron.Register(j)
	}

	// The retention is read on every run, an administrator changes it.
	since := func() time.Time { return time.Now().UTC().Add(-rt.FileLimits.Get().RetainDeleted) }
	hourly := cron.EveryInterval(time.Hour)

	register(cron.Job{
		Name:     "blob-release",
		Schedule: hourly,
		Run: func(ctx context.Context) error {
			n, err := attachment.Sweep(ctx, rt, since())
			if n > 0 {
				rt.Log.Info("files: deleted the bytes of files gone for good", "blobs", n)
			}

			return err
		},
	})

	// Not a sweep: how a node learns a type declared on another one.
	register(cron.Job{
		Name:     "custom-types",
		Schedule: cron.EveryInterval(time.Minute),
		Run:      func(ctx context.Context) error { return customtype.Load(ctx, rt) },
	})

	// How a node learns the settings saved on another one.
	register(cron.Job{
		Name:     "settings",
		Schedule: cron.EveryInterval(time.Minute),
		Run:      func(ctx context.Context) error { return setting.Load(ctx, rt) },
	})

	register(cron.Job{
		Name:     "schedules",
		Schedule: cron.EveryInterval(time.Minute),
		Run:      func(ctx context.Context) error { return schedule.Tick(ctx, rt) },
	})

	register(cron.Job{
		Name:     "undo-journal",
		Schedule: hourly,
		Run: func(ctx context.Context) error {
			_, err := rt.Store.Graph.PruneChanges(ctx, since())

			return err
		},
	})

	register(cron.Job{
		Name:     "sessions",
		Schedule: hourly,
		Run: func(ctx context.Context) error {
			_, err := rt.Store.Session.DeleteExpired(ctx, time.Now().UTC())

			return err
		},
	})

	register(cron.Job{
		Name:     "password-resets",
		Schedule: hourly,
		Run: func(ctx context.Context) error {
			_, err := rt.Store.PasswordReset.DeleteExpired(ctx, time.Now().UTC())

			return err
		},
	})
}
