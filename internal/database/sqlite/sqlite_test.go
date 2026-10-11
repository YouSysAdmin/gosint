// Gosint, Copyright (c) 2026 YouSysAdmin

package sqlite_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/sqlite"
)

// A directory name holding URI characters still opens the file there,
// not a truncated or decoded path somewhere else.
func TestAPathWithURICharactersOpens(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "case ?42 #a %41")
	path := filepath.Join(dir, "gosint.db")

	db, err := sqlite.Open(t.Context(), path, database.Pool{})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	if _, err := os.Stat(path); err != nil {
		t.Errorf("the database is not where it was asked for: %v", err)
	}
}
