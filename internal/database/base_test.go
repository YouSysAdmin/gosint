// Gosint, Copyright (c) 2026 YouSysAdmin

package database_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

func TestRebind(t *testing.T) {
	got := database.Rebind("SELECT a FROM t WHERE b = ? AND c IN (?, ?)")
	want := "SELECT a FROM t WHERE b = $1 AND c IN ($2, $3)"
	if got != want {
		t.Errorf("Rebind = %q, want %q", got, want)
	}
}

// One query text, both engines: placeholders, a time column and a
// boolean have to round-trip identically.
func TestTheSameQueryRunsOnEveryBackend(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		b := database.NewBase(db)
		ctx := t.Context()
		id := ids.New()
		now := time.Now().UTC().Truncate(time.Microsecond)

		_, err := b.Exec(ctx, `INSERT INTO users (id, email, email_sealed, password_hash, is_active, created_at, updated_at)
			VALUES (?, ?, '', ?, ?, ?, ?)`, id, "a@example.com", "x", true, now, now)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		var (
			email   string
			active  bool
			created time.Time
		)

		err = b.QueryRow(ctx, `SELECT email, is_active, created_at FROM users WHERE id = ?`, id).
			Scan(&email, &active, &created)
		if err != nil {
			t.Fatalf("select: %v", err)
		}

		if email != "a@example.com" || !active || !created.Equal(now) {
			t.Errorf("got (%q, %v, %s), want (a@example.com, true, %s)", email, active, created, now)
		}
	})
}

func TestUniqueViolationIsRecognizedOnEveryBackend(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		b := database.NewBase(db)
		ctx := t.Context()
		now := time.Now().UTC()
		insert := `INSERT INTO users (id, email, email_sealed, password_hash, created_at, updated_at) VALUES (?, ?, '', ?, ?, ?)`

		if _, err := b.Exec(ctx, insert, ids.New(), "dup@example.com", "x", now, now); err != nil {
			t.Fatalf("first insert: %v", err)
		}

		_, err := b.Exec(ctx, insert, ids.New(), "dup@example.com", "x", now, now)
		if !database.UniqueViolation(err) {
			t.Errorf("second insert: UniqueViolation(%v) = false", err)
		}

		if database.UniqueViolation(errors.New("plain")) {
			t.Error("a plain error was reported as a unique violation")
		}
	})
}

func TestTxRollsBackOnError(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		b := database.NewBase(db)
		ctx := t.Context()
		now := time.Now().UTC()
		boom := errors.New("boom")

		err := b.Tx(ctx, func(tx *database.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO users (id, email, email_sealed, password_hash, created_at, updated_at)
				VALUES (?, ?, '', ?, ?, ?)`, ids.New(), "tx@example.com", "x", now, now)
			if err != nil {
				return err
			}

			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("Tx = %v, want boom", err)
		}

		var n int
		if err := b.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}

		if n != 0 {
			t.Errorf("%d rows survived the rollback", n)
		}
	})
}

// postgres refuses a non-uuid against a uuid column, sqlite just finds
// nothing. Either way the caller must see "no such row", not a 500.
func TestMalformedIDIsRecognized(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		b := database.NewBase(db)

		var email string
		err := b.QueryRow(t.Context(), `SELECT email FROM users WHERE id = ?`, "banana").Scan(&email)
		switch db.Dialect() {
		case database.Postgres:
			if !database.MalformedID(err) {
				t.Errorf("MalformedID(%v) = false", err)
			}
		default:
			if !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("err = %v, want sql.ErrNoRows", err)
			}
		}
	})
}
