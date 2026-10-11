// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/schema"
	"github.com/yousysadmin/gosint/internal/domain/sys"
)

func newVaultCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "vault",
		Short: "Initialize the master key or change its unseal key",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Wrap the master key under an unseal key",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withDatabase(cmd, func(ctx context.Context, cfg *env.Config, db database.Database) error {
				return vaultInit(ctx, cmd, cfg, db)
			})
		},
	}, &cobra.Command{
		Use:   "rekey",
		Short: "Change the unseal key, the data stays as it is",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return withDatabase(cmd, func(ctx context.Context, _ *env.Config, db database.Database) error {
				return vaultRekey(ctx, cmd, db)
			})
		},
	})

	return cmd
}

// withDatabase opens the configured database at the current schema.
func withDatabase(cmd *cobra.Command, fn func(context.Context, *env.Config, database.Database) error) error {
	path, _ := cmd.Flags().GetString("config")
	cfg, err := env.Load(path)
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return usage("invalid config: %w", err)
	}

	ctx := cmd.Context()
	db, err := backend.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}

	defer db.Close() //nolint:errcheck // closing on exit

	if err := schema.Migrate(ctx, db); err != nil {
		return err
	}

	return fn(ctx, cfg, db)
}

func vaultInit(ctx context.Context, cmd *cobra.Command, cfg *env.Config, db database.Database) error {
	// A key given in the config for development is the one wrapped.
	v := vault.NewSealed()
	if cfg.Vault.MasterKey != "" {
		key, err := vault.ParseKey(cfg.Vault.MasterKey)
		if err != nil {
			return err
		}

		if err := v.Unseal(key); err != nil {
			return err
		}

		cmd.Println("The key in vault.master_key will be wrapped. Remove it from the config after.")
	}

	unsealKey, err := newPrompter(cmd).askTwice("New unseal key")
	if err != nil {
		return err
	}

	if err := sys.Initialize(ctx, sys.NewStore(db), v, unsealKey); err != nil {
		if errors.Is(err, sys.ErrShortUnsealKey) {
			return usage("%w", err)
		}

		return err
	}

	cmd.Println("Initialized. Start the server and unseal it with this key. It cannot be recovered, keep it safe.")

	return nil
}

func vaultRekey(ctx context.Context, cmd *cobra.Command, db database.Database) error {
	p := newPrompter(cmd)
	current, err := p.ask("Current unseal key")
	if err != nil {
		return err
	}

	next, err := p.askTwice("New unseal key")
	if err != nil {
		return err
	}

	if err := sys.Rekey(ctx, sys.NewStore(db), current, next); err != nil {
		if errors.Is(err, sys.ErrShortUnsealKey) || errors.Is(err, vault.ErrUnsealKey) {
			return usage("%w", err)
		}

		return err
	}

	cmd.Println("The unseal key is changed.")

	return nil
}

// prompter reads unseal keys without echo from a terminal, or lines
// from a pipe through one buffer.
type prompter struct {
	cmd  *cobra.Command
	fd   int
	tty  bool
	pipe *bufio.Reader
}

func newPrompter(cmd *cobra.Command) *prompter {
	p := &prompter{cmd: cmd}
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		p.fd, p.tty = int(f.Fd()), true

		return p
	}

	p.pipe = bufio.NewReader(in)

	return p
}

// askTwice reads a new unseal key and its confirmation.
func (p *prompter) askTwice(prompt string) (string, error) {
	first, err := p.ask(prompt)
	if err != nil {
		return "", err
	}

	again, err := p.ask("Again")
	if err != nil {
		return "", err
	}

	if first != again {
		return "", usage("the two unseal keys differ")
	}

	return first, nil
}

func (p *prompter) ask(prompt string) (string, error) {
	p.cmd.PrintErr(prompt + ": ")

	if p.tty {
		b, err := term.ReadPassword(p.fd)
		p.cmd.PrintErrln()

		return string(b), err
	}

	line, err := p.pipe.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read unseal key: %w", err)
	}

	return strings.TrimRight(line, "\r\n"), nil
}
