// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/gosint/internal/blob/sealed"
	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/live"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/oidc"
	"github.com/yousysadmin/gosint/internal/core/release"
	"github.com/yousysadmin/gosint/internal/core/tlsconf"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/transformdir"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/postgres"
	"github.com/yousysadmin/gosint/internal/database/schema"
	"github.com/yousysadmin/gosint/internal/domain/customtype"
	"github.com/yousysadmin/gosint/internal/domain/setting"
	"github.com/yousysadmin/gosint/internal/server"
	"github.com/yousysadmin/gosint/internal/transforms"
	"github.com/yousysadmin/gosint/pkg"
	"github.com/yousysadmin/gosint/web"
)

// shutdownTimeout bounds how long in-flight requests get on stop.
const shutdownTimeout = 15 * time.Second

// releaseTimeout bounds the request asking GitHub for a release.
const releaseTimeout = 15 * time.Second

func newServeCmd() *cobra.Command {
	var skipMigrate bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the API, the console and the workers",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, _ := cmd.Flags().GetString("config")

			return runServe(cmd.Context(), path, skipMigrate)
		},
	}

	cmd.Flags().BoolVar(&skipMigrate, "skip-migrate", false,
		"do not apply migrations, refuse to start on an outdated schema instead")

	return cmd
}

func runServe(ctx context.Context, configPath string, skipMigrate bool) error {
	cfg, err := env.Load(configPath)
	if err != nil {
		return err
	}

	return RunNode(ctx, cfg, NodeOptions{SkipMigrate: skipMigrate})
}

// NodeOptions changes how RunNode starts, for a program embedding a
// node such as the desktop app.
type NodeOptions struct {
	// SkipMigrate verifies the schema instead of migrating it.
	SkipMigrate bool

	// Listener, when set, is served in place of server.addr.
	Listener net.Listener

	// Ready is called once the server is about to answer.
	Ready func()
}

// RunNode runs the API, the console and the worker on cfg until ctx
// ends or a signal arrives.
func RunNode(ctx context.Context, cfg *env.Config, opts NodeOptions) error {
	if err := cfg.Validate(); err != nil {
		return usage("invalid config: %w", err)
	}

	log, err := buildLogger(cfg.Logging)
	if err != nil {
		return fmt.Errorf("logger: %w", err)
	}

	slog.SetDefault(log)
	log.Info("starting", "app", pkg.AppName, "version", pkg.Version, "config", cfg.Source)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := backend.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}

	defer db.Close() //nolint:errcheck // closing on exit

	if opts.SkipMigrate {
		err = schema.RequireCurrent(ctx, db)
	} else {
		err = schema.Migrate(ctx, db)
	}

	if err != nil {
		return err
	}

	v := vault.NewSealed()
	if err := bootVault(ctx, cfg, db, v, log); err != nil {
		return err
	}

	nodeID := newNodeID()
	events := eventbus.New()
	defer events.Close()

	rt := &env.Runtime{
		Config:     cfg,
		Log:        log,
		DB:         db,
		Store:      backend.BindStore(db, v),
		Vault:      v,
		Events:     events,
		Transforms: transform.NewRegistry(transforms.All()...),

		Tools:        tool.NewRegistry(transforms.Tools()...),
		ToolSettings: live.New(tool.Settings{}),
		WorkerTuning: live.New(worker.DefaultTuning()),
		CacheDir:     live.New(cfg.DefaultCacheDir()),

		WakeWorker: func() {},
		Blobs:      sealed.NewSwitch(v),
		FileLimits: live.New(env.DefaultFileLimits()),
		Mail:       mailer.NewSwitch(log),
		OAuth:      oidc.NewRegistry(cfg.Server.PublicURL),
		Cron:       cron.New(log),
	}

	if cfg.Console.UpdateCheck {
		rt.Release = &release.Checker{
			Current: pkg.Version,
			Client:  &http.Client{Timeout: releaseTimeout},
			Agent:   pkg.AppName + "/" + pkg.Version,
		}
	}

	rt.TargetHTTP = transform.TargetHTTPClient(pkg.AppName+"/"+pkg.Version, func() bool {
		return rt.ToolSettings.Get().Get(tool.WebName).AllowPrivate
	})

	if dir := cfg.TransformsDir(); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return fmt.Errorf("transforms directory: %w", err)
		}

		files := transformdir.New(dir, types.Builtin(), log)
		for _, e := range transforms.All() {
			files.Reserved = append(files.Reserved, e.Meta().Name)
		}

		rt.Transforms.SetSource(files.Transforms)
		log.Info("transform files", "dir", dir, "loaded", len(files.Transforms()))
	}

	if p, ok := db.(*postgres.Postgres); ok {
		postgres.NewRelay(p, cfg.Database.DSN, nodeID, events, v, log).Start(ctx)
	}

	if n, err := rt.Store.Session.DeleteExpired(ctx, time.Now().UTC()); err != nil {
		log.Warn("could not sweep expired sessions", "err", err)
	} else if n > 0 {
		log.Info("swept expired sessions", "count", n)
	}

	// The custom entity types and the settings, declared in the database
	// and sealed, are read before anything uses them, at once when the
	// vault is open, at the unseal otherwise.
	sealedAtBoot := v.IsSealed()
	if !sealedAtBoot {
		if err := customtype.Load(ctx, rt); err != nil {
			return fmt.Errorf("custom types: %w", err)
		}

		if err := setting.Load(ctx, rt); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
	}

	// The scheduled sweeps, skipped while sealed. Stopping the context
	// ends the scheduling, a job in flight finishes under the shutdown
	// timeout.
	registerJobs(rt)
	go rt.Cron.Start(ctx)

	// The worker starts once the vault is open and the settings are
	// read: a sealed server cannot open what a scan reads, and the
	// settings say whether it runs at all.
	w := newWorker(rt, nodeID)
	rt.WakeWorker = w.Wake
	workerDone := make(chan struct{})

	go func() {
		defer close(workerDone)

		select {
		case <-v.Ready():
		case <-ctx.Done():
			return
		}

		if sealedAtBoot {
			if err := customtype.Load(ctx, rt); err != nil {
				log.Error("custom types could not be loaded, the job tries again in a minute", "err", err)
			}

			if err := setting.Load(ctx, rt); err != nil {
				log.Error("settings could not be loaded, the job tries again in a minute", "err", err)
			}
		}

		log.Info("worker started", "node", nodeID, "enabled", rt.WorkerTuning.Get().Enabled)
		w.Run(ctx)
	}()

	// On an early return the worker must still hand its scans back
	// before the deferred closes take the database away.
	defer func() {
		stop()
		<-workerDone
	}()

	// Stop first: on an early return nothing else has cancelled the
	// context yet and the wait would sit out its whole timeout.
	defer func() {
		stop()
		rt.Cron.Wait(shutdownTimeout)
	}()

	tlsCfg, err := tlsconf.Build(cfg)
	if err != nil {
		return fmt.Errorf("tls: %w", err)
	}

	if tlsCfg != nil {
		log.Info("tls enabled", "mode", cfg.Server.TLS.Mode, "host", cfg.TLSHost())
	}

	srv, err := server.New(server.Options{Runtime: rt, Console: web.FS(), TLS: tlsCfg})
	if err != nil {
		return err
	}

	if !web.Available() {
		log.Warn("no console embedded in this build - run `task web` before building")
	}

	errc := make(chan error, 1)
	go func() {
		if opts.Listener != nil {
			errc <- srv.Serve(opts.Listener)

			return
		}

		errc <- srv.Start()
	}()

	if opts.Ready != nil {
		opts.Ready()
	}

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")

	// The event streams hold their connections open until the bus
	// closes, and the server waits for open connections, so the bus
	// goes first or every shutdown sits out the timeout.
	events.Close()

	if err := srv.Shutdown(shutdownTimeout); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// The worker hands running scans back before the database closes.
	if workerDone != nil {
		<-workerDone
	}

	if err := <-errc; err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
