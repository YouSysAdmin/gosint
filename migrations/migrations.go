// Gosint, Copyright (c) 2026 YouSysAdmin

// Package migrations embeds the schema, one tree per backend.
//
// It lives at the repository root, beside the SQL, because go:embed
// cannot reach outside its own package directory.
//
// One file per table, named for the table, and the SAME file names in
// both trees: postgres/00001_users.sql and sqlite/00001_users.sql
// describe the same table in each engine's types.
// TestBothSchemaTreesMatch keeps the two in step.
//
// An applied file is FROZEN. goose never looks at a version it has
// run, so a change to an existing table is a new file at the next
// number in both trees.
package migrations

import (
	"embed"
	"fmt"
	"io/fs"

	"github.com/yousysadmin/gosint/internal/database"
)

//go:embed postgres/*.sql sqlite/*.sql
var files embed.FS

// For returns the migration tree for a dialect.
func For(d database.Dialect) (fs.FS, error) {
	switch d {
	case database.Postgres, database.SQLite:
		return fs.Sub(files, string(d))
	default:
		return nil, fmt.Errorf("migrations: unknown dialect %q", d)
	}
}
