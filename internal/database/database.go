// Gosint, Copyright (c) 2026 YouSysAdmin

// Package database declares the abstraction every persistence backend
// implements, plus the helpers stores share.
//
// Two backends exist, sqlite and postgres, and they run the SAME query
// text. Stores write `?` placeholders and Base rewrites them for the
// backend, which is the only dialect difference a store may see. A
// query that would need to branch on the dialect is written in the
// subset both engines speak instead.
package database

import "database/sql"

// Dialect names a backend.
type Dialect string

// The supported dialects.
const (
	SQLite   Dialect = "sqlite"
	Postgres Dialect = "postgres"
)

// Database is the backend handle the runtime holds.
type Database interface {
	// Close releases backend resources. Idempotent.
	Close() error

	// DB returns the underlying *sql.DB. Pool tuning is the backend's
	// job, so callers do not change pool settings.
	DB() *sql.DB

	// Dialect says which engine is behind DB.
	Dialect() Dialect
}
