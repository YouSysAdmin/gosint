// Gosint, Copyright (c) 2026 YouSysAdmin

// Package dbtest runs store tests against every backend.
//
// sqlite always runs, on a fresh file per test. postgres runs when
// GOSINT_TEST_PG_DSN names a database, each test in a schema of its
// own that is dropped afterwards.
package dbtest

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/postgres"
	"github.com/yousysadmin/gosint/internal/database/schema"
	"github.com/yousysadmin/gosint/internal/database/sqlite"
)

// PostgresDSNEnv names the postgres database the tests may use.
const PostgresDSNEnv = "GOSINT_TEST_PG_DSN"

// Each runs fn once per available backend, each on a migrated, empty
// database.
func Each(t *testing.T, fn func(t *testing.T, db database.Database)) {
	t.Helper()

	t.Run(string(database.SQLite), func(t *testing.T) {
		fn(t, SQLite(t))
	})

	t.Run(string(database.Postgres), func(t *testing.T) {
		fn(t, Postgres(t))
	})
}

// SQLite opens a migrated sqlite database in a temp directory.
func SQLite(t *testing.T) database.Database {
	t.Helper()

	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "test.db"), database.Pool{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })
	migrate(t, db)

	return db
}

// Postgres opens a migrated postgres database in a throwaway schema,
// or skips when GOSINT_TEST_PG_DSN is unset.
func Postgres(t *testing.T) database.Database {
	t.Helper()

	dsn := os.Getenv(PostgresDSNEnv)
	if dsn == "" {
		t.Skipf("%s is not set", PostgresDSNEnv)
	}

	name := "t_" + strings.ReplaceAll(ids.New(), "-", "")

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	defer admin.Close() //nolint:errcheck // test helper

	// The name is built from a minted uuid, never from input.
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+name); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	t.Cleanup(func() {
		c, err := sql.Open("pgx", dsn)
		if err != nil {
			return
		}

		defer c.Close() //nolint:errcheck // test helper

		_, _ = c.ExecContext(context.Background(), "DROP SCHEMA "+name+" CASCADE")
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse %s: %v", PostgresDSNEnv, err)
	}

	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()

	db, err := postgres.Open(t.Context(), u.String(), database.Pool{})
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	t.Cleanup(func() { _ = db.Close() })
	migrate(t, db)

	return db
}

func migrate(t *testing.T, db database.Database) {
	t.Helper()

	if err := schema.Migrate(t.Context(), db); err != nil {
		t.Fatalf("migrate %s: %v", db.Dialect(), err)
	}
}
