// Gosint, Copyright (c) 2026 YouSysAdmin

// Package schema applies the embedded migrations through goose and
// checks that a database matches the binary.
package schema

import (
	"context"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/migrations"
)

// Migrate applies every pending migration.
//
// On postgres the run holds a session advisory lock, so several nodes
// booting together against one database apply the schema once.
func Migrate(ctx context.Context, d database.Database) error {
	p, err := provider(d, true)
	if err != nil {
		return err
	}

	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

// RequireCurrent refuses a database whose schema is older than the
// binary, for a node started without migrating.
func RequireCurrent(ctx context.Context, d database.Database) error {
	p, err := provider(d, false)
	if err != nil {
		return err
	}

	current, target, err := p.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	if current < target {
		return fmt.Errorf("the database schema is at version %d and this binary needs %d - start one node without --skip-migrate",
			current, target)
	}

	return nil
}

func provider(d database.Database, locked bool) (*goose.Provider, error) {
	fsys, err := migrations.For(d.Dialect())
	if err != nil {
		return nil, err
	}

	var (
		dialect goose.Dialect
		opts    []goose.ProviderOption
	)

	switch d.Dialect() {
	case database.Postgres:
		dialect = goose.DialectPostgres
		if locked {
			locker, err := lock.NewPostgresSessionLocker()
			if err != nil {
				return nil, fmt.Errorf("migration lock: %w", err)
			}

			opts = append(opts, goose.WithSessionLocker(locker))
		}
	case database.SQLite:
		dialect = goose.DialectSQLite3
	default:
		return nil, fmt.Errorf("schema: unknown dialect %q", d.Dialect())
	}

	p, err := goose.NewProvider(dialect, d.DB(), fsys, opts...)
	if err != nil {
		return nil, fmt.Errorf("migration provider: %w", err)
	}

	return p, nil
}
