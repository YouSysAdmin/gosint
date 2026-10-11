// Gosint, Copyright (c) 2026 YouSysAdmin

// Package postgres is the team backend: the pgx driver through
// database/sql.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/yousysadmin/gosint/internal/database"
)

// Postgres implements database.Database over a pgx connection pool.
type Postgres struct {
	db *sql.DB
}

// Open connects to dsn (postgres://user:pass@host:5432/dbname?sslmode=...)
// and verifies the connection.
func Open(ctx context.Context, dsn string, pool database.Pool) (*Postgres, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	pool.Apply(db)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("ping postgres %q: %w", redactDSN(dsn), err)
	}

	slog.Info("postgres open", "dsn", redactDSN(dsn))

	return &Postgres{db: db}, nil
}

// Close releases the pool.
func (p *Postgres) Close() error { return p.db.Close() }

// DB is the pool handle.
func (p *Postgres) DB() *sql.DB { return p.db }

// Dialect reports postgres.
func (p *Postgres) Dialect() database.Dialect { return database.Postgres }

// redactDSN masks the password so the DSN is safe to log.
func redactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Scheme == "" {
		return "(redacted dsn)"
	}

	return u.Redacted()
}
