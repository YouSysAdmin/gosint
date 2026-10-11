// Gosint, Copyright (c) 2026 YouSysAdmin

package migrations_test

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/migrations"
)

// A migration added to one engine and forgotten in the other gives the
// two installs different schemas with no error anywhere.
func TestBothSchemaTreesMatch(t *testing.T) {
	names := func(d database.Dialect) []string {
		fsys, err := migrations.For(d)
		if err != nil {
			t.Fatal(err)
		}

		out, err := fs.Glob(fsys, "*.sql")
		if err != nil {
			t.Fatal(err)
		}

		return out
	}

	pg, lite := names(database.Postgres), names(database.SQLite)
	if len(pg) == 0 {
		t.Fatal("no postgres migrations embedded")
	}

	if !slices.Equal(pg, lite) {
		t.Errorf("migration trees differ:\npostgres: %v\nsqlite:   %v", pg, lite)
	}
}
