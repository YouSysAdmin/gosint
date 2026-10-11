// Gosint, Copyright (c) 2026 YouSysAdmin

// Package passwordreset persists the single-use tokens behind the
// forgot-password flow. The routes hang off /api/auth, this is storage.
package passwordreset

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	prmodel "github.com/yousysadmin/gosint/internal/models/passwordreset"
)

// Store persists reset tokens, the requesting address sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func ipAAD(id string) string { return database.AAD("password_resets", "request_ip", id) }

//nolint:gosec // column names, no credential
const tokenSelect = `
SELECT id, user_id, token_hash, expires_at, used_at, created_at, request_ip
FROM password_resets`

// GetByHash is the redemption lookup. Not user scoped, the token IS
// the claim of identity.
func (s *Store) GetByHash(ctx context.Context, hash string) (*prmodel.Token, error) {
	t, err := scanToken(s.QueryRow(ctx, tokenSelect+` WHERE token_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	t.RequestIP, err = s.codec.OpenText(ipAAD(t.ID), t.RequestIP)

	return t, err
}

// Put inserts a token.
func (s *Store) Put(ctx context.Context, t *prmodel.Token) error {
	ip, err := s.codec.SealText(ipAAD(t.ID), t.RequestIP)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO password_resets (id, user_id, token_hash, expires_at, used_at, created_at, request_ip)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.UserID, t.TokenHash, t.ExpiresAt, database.NullTime(t.UsedAt), t.CreatedAt, ip)

	return err
}

// MarkUsed burns one token and reports whether this call is the one
// that burned it: the conditional update is the single-use guarantee,
// and it only counts if the caller checks.
func (s *Store) MarkUsed(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := s.Exec(ctx, `UPDATE password_resets SET used_at = ? WHERE id = ? AND used_at IS NULL`, at, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// InvalidateForUser burns every outstanding token of a user, after a
// reset or any other password change.
func (s *Store) InvalidateForUser(ctx context.Context, userID string, at time.Time) error {
	_, err := s.Exec(ctx, `UPDATE password_resets SET used_at = ? WHERE user_id = ? AND used_at IS NULL`, at, userID)

	return err
}

// RecentRequestIPs is the address behind each link the account was
// issued since a moment. The caller judges them, an IPv6 one by its
// /64.
func (s *Store) RecentRequestIPs(ctx context.Context, userID string, since time.Time) ([]string, error) {
	rows, err := s.Query(ctx, `SELECT id, request_ip FROM password_resets WHERE user_id = ? AND created_at >= ?`, userID, since)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []string
	for rows.Next() {
		var id, ip string
		if err := rows.Scan(&id, &ip); err != nil {
			return nil, err
		}

		ip, err := s.codec.OpenText(ipAAD(id), ip)
		if err != nil {
			return nil, err
		}

		out = append(out, ip)
	}

	return out, rows.Err()
}

// DeleteExpired purges spent and stale rows.
func (s *Store) DeleteExpired(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM password_resets WHERE expires_at < ? OR used_at IS NOT NULL`, before)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func scanToken(r interface{ Scan(...any) error }) (*prmodel.Token, error) {
	var t prmodel.Token
	var used sql.NullTime
	if err := r.Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &used, &t.CreatedAt, &t.RequestIP); err != nil {
		return nil, err
	}

	if used.Valid {
		t.UsedAt = &used.Time
	}

	return &t, nil
}
