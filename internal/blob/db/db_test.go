// Gosint, Copyright (c) 2026 YouSysAdmin

package db_test

import (
	"testing"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/blob/blobtest"
	"github.com/yousysadmin/gosint/internal/blob/db"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

func TestContract(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, d database.Database) {
		blobtest.Run(t, func(t *testing.T) blob.Store {
			// One database per test, one empty table per subtest.
			if _, err := d.DB().ExecContext(t.Context(), `DELETE FROM blobs`); err != nil {
				t.Fatal(err)
			}

			return db.New(d, t.TempDir())
		})
	})
}
