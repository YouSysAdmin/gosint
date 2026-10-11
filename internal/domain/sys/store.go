// Gosint, Copyright (c) 2026 YouSysAdmin

// Package sys holds the server's own state: the master key wrapped
// under the operator's unseal key, and the routes that seal and unseal.
package sys

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
)

// Row is the stored seal.
type Row struct {
	Wrapped  string
	KeyCheck string
}

// Store reads and writes the one vault_seal row.
type Store struct{ database.Base }

// NewStore binds the store to db.
func NewStore(db database.Database) *Store { return &Store{database.NewBase(db)} }

// Get is the seal, nil before the server was initialized.
func (s *Store) Get(ctx context.Context) (*Row, error) {
	var r Row
	err := s.QueryRow(ctx, `SELECT wrapped, key_check FROM vault_seal WHERE id = 1`).Scan(&r.Wrapped, &r.KeyCheck)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &r, nil
}

// Create writes the first seal and reports whether this call did, so
// two nodes initializing together keep one key.
func (s *Store) Create(ctx context.Context, r Row, now time.Time) (bool, error) {
	res, err := s.Exec(ctx, `
		INSERT INTO vault_seal (id, wrapped, key_check, created_at, updated_at) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`, r.Wrapped, r.KeyCheck, now, now)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n == 1, err
}

// HoldsSealedData reports whether rows were sealed under some key: an
// account is the first thing every install seals.
func (s *Store) HoldsSealedData(ctx context.Context) (bool, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)

	return n > 0, err
}

// Rewrap replaces the wrapped key, the master key itself unchanged.
func (s *Store) Rewrap(ctx context.Context, wrapped string, now time.Time) error {
	_, err := s.Exec(ctx, `UPDATE vault_seal SET wrapped = ?, updated_at = ? WHERE id = 1`, wrapped, now)

	return err
}
