// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/env"
	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database"
	flowdomain "github.com/yousysadmin/gosint/internal/domain/flow"
	"github.com/yousysadmin/gosint/internal/domain/sys"
	"github.com/yousysadmin/gosint/internal/domain/transformtemplate"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	"github.com/yousysadmin/gosint/pkg"
)

// bootVault hands v its master key when the config gives a way to,
// and otherwise leaves it sealed for the console or the CLI.
func bootVault(ctx context.Context, cfg *env.Config, db database.Database, v *vault.Vault, log *slog.Logger) error {
	st := sys.NewStore(db)
	row, err := st.Get(ctx)
	if err != nil {
		return err
	}

	if cfg.Vault.MasterKey != "" {
		key, err := vault.ParseKey(cfg.Vault.MasterKey)
		if err != nil {
			return err
		}

		if row != nil && !sys.Matches(row, key) {
			return sys.ErrKeyMismatch
		}

		log.Warn("vault.master_key unseals at boot, the key sits in clear in the config, for development only")

		return v.Unseal(key)
	}

	unsealKey, err := configUnsealKey(cfg)
	if err != nil {
		return err
	}

	switch {
	case unsealKey != "" && row == nil:
		if err := sys.Initialize(ctx, st, v, unsealKey); err != nil {
			return fmt.Errorf("vault: %w", err)
		}

		log.Warn("vault initialized from the configured unseal key, the data is only as safe as that unseal key")
	case unsealKey != "":
		if err := sys.Unseal(ctx, st, v, unsealKey); err != nil {
			return fmt.Errorf("vault: %w", err)
		}

		log.Warn("vault unsealed from the configured unseal key, the data is only as safe as that unseal key")
	case row == nil:
		log.Warn("sealed: no master key yet, initialize with an unseal key in the console or with `gosint vault init`")
	default:
		log.Warn("sealed: enter the unseal key in the console")
	}

	return nil
}

// configUnsealKey is the unseal key the config gives for an
// unattended start, empty when it gives none.
func configUnsealKey(cfg *env.Config) (string, error) {
	if cfg.Vault.UnsealKeyFile == "" {
		return cfg.Vault.UnsealKey, nil
	}

	b, err := os.ReadFile(cfg.Vault.UnsealKeyFile)
	if err != nil {
		return "", fmt.Errorf("vault.unseal_key_file: %w", err)
	}

	return strings.TrimRight(string(b), "\r\n"), nil
}

// newNodeID names this process to the queue and the event relay.
func newNodeID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "node"
	}

	return fmt.Sprintf("%s-%d-%s", host, os.Getpid(), ids.New()[24:])
}

// newWorker builds the scan runner with a handler per scan kind.
func newWorker(rt *env.Runtime, nodeID string) *worker.Worker {
	w := worker.New(worker.Config{
		Scans:  rt.Store.Scan,
		Logs:   rt.Store.Log,
		Bus:    rt.Events,
		Log:    rt.Log,
		NodeID: nodeID,
		Tune:   rt.WorkerTuning.Get,
		Paused: rt.Vault.IsSealed,
	})

	runner := transform.RunnerConfig{
		Lookup: transform.ByName(rt.Transforms),
		Types:  types.Builtin(),
		Graph:  rt.Store.Graph,
		Secrets: func(ctx context.Context, ownerID, ref string) (string, bool, error) {
			return vaultkey.Secret(ctx, rt, ownerID, ref)
		},
		HTTP:     transform.DefaultHTTPClient(pkg.AppName + "/" + pkg.Version),
		Web:      rt.TargetHTTP,
		Resolver: net.DefaultResolver,
		CacheDir: rt.CacheDir.Get,

		Tools: rt.ToolSettings.Get,
		Files: rt.Store.Attachment,
	}
	w.Handle(transform.ScanKind, transform.Handler(runner))

	w.Handle(coreflow.ScanKind, coreflow.Handler(coreflow.RunnerConfig{
		RunnerConfig: runner, Registry: rt.Transforms, Definition: flowdomain.Definition(rt),
	}))

	// A template is a transform found by its stored id instead.
	runner.Lookup = transformtemplate.Lookup(rt)
	w.Handle(transformtemplate.ScanKind, transform.Handler(runner))

	return w
}
