// Gosint, Copyright (c) 2026 YouSysAdmin

// Package backend opens the database the config names. It is the one
// place that imports both drivers.
package backend

import (
	"context"
	"fmt"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/postgres"
	"github.com/yousysadmin/gosint/internal/database/sqlite"
)

// Open connects to the configured backend.
func Open(ctx context.Context, cfg env.DatabaseConfig) (database.Database, error) {
	pool := database.Pool{
		MaxOpen:     cfg.MaxOpenConns,
		MaxIdle:     cfg.MaxIdleConns,
		MaxLifetime: cfg.ConnMaxLifetime,
		MaxIdleTime: cfg.ConnMaxIdleTime,
	}

	switch cfg.Driver {
	case env.DriverSQLite:
		return sqlite.Open(ctx, cfg.DSN, pool)
	case env.DriverPostgres:
		return postgres.Open(ctx, cfg.DSN, pool)
	default:
		return nil, fmt.Errorf("unknown database driver %q", cfg.Driver)
	}
}
