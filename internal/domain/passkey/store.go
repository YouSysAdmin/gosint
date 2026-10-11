// Gosint, Copyright (c) 2026 YouSysAdmin

// Package passkey persists enrolled WebAuthn credentials and the spent
// ceremony challenges.
package passkey

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	pkmodel "github.com/yousysadmin/gosint/internal/models/passkey"
)

// Store persists passkeys, the name the person gave one sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func nameAAD(id string) string { return database.AAD("user_passkeys", "name", id) }

//nolint:gosec // column names, no credential
const passkeySelect = `
SELECT id, user_id, credential_id, name, credential, created_at, last_used_at
FROM user_passkeys`

// ListForUser returns the account's passkeys, newest first.
func (s *Store) ListForUser(ctx context.Context, userID string) ([]pkmodel.Passkey, error) {
	rows, err := s.Query(ctx, passkeySelect+` WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []pkmodel.Passkey{}
	for rows.Next() {
		m, err := s.scanPasskey(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *m)
	}

	return out, rows.Err()
}

// GetByCredential resolves the credential the authenticator asserted.
// Not user scoped: in a discoverable login nobody has claimed an
// identity yet, the credential id IS the claim and the signature over
// it is what makes it true.
func (s *Store) GetByCredential(ctx context.Context, credentialID string) (*pkmodel.Passkey, error) {
	m, err := s.scanPasskey(s.QueryRow(ctx, passkeySelect+` WHERE credential_id = ?`, credentialID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return m, err
}

// Put inserts a newly enrolled passkey.
func (s *Store) Put(ctx context.Context, m *pkmodel.Passkey) error {
	name, err := s.codec.SealText(nameAAD(m.ID), m.Name)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO user_passkeys (id, user_id, credential_id, name, credential, created_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.UserID, m.CredentialID, name, m.Credential, m.CreatedAt, database.NullTime(m.LastUsedAt))

	return err
}

// RecordUse saves the credential after an assertion, since the sign
// counter advanced, and stamps the time.
func (s *Store) RecordUse(ctx context.Context, id, credential string, at time.Time) error {
	_, err := s.Exec(ctx, `UPDATE user_passkeys SET credential = ?, last_used_at = ? WHERE id = ?`, credential, at, id)

	return err
}

// Rename changes the label. Scoped by user, so somebody else's passkey
// reads as missing.
func (s *Store) Rename(ctx context.Context, userID, id, name string) (bool, error) {
	sealed, err := s.codec.SealText(nameAAD(id), name)
	if err != nil {
		return false, err
	}

	res, err := s.Exec(ctx, `UPDATE user_passkeys SET name = ? WHERE user_id = ? AND id = ?`, sealed, userID, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Delete removes one passkey of the user.
func (s *Store) Delete(ctx context.Context, userID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM user_passkeys WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// DeleteAllForUser removes every passkey of the account, for an admin
// whose user lost the only device that could sign in.
func (s *Store) DeleteAllForUser(ctx context.Context, userID string) (int, error) {
	res, err := s.Exec(ctx, `DELETE FROM user_passkeys WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}

	n, err := res.RowsAffected()

	return int(n), err
}

// CountForUser is how many the account holds.
func (s *Store) CountForUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM user_passkeys WHERE user_id = ?`, userID).Scan(&n)

	return n, err
}

// SpendChallenge records a finished ceremony's challenge and reports
// whether this was its first use. Expired rows are pruned on the way.
func (s *Store) SpendChallenge(ctx context.Context, challenge string, expiresAt, now time.Time) (bool, error) {
	if _, err := s.Exec(ctx, `DELETE FROM passkey_challenges WHERE expires_at < ?`, now); err != nil {
		return false, err
	}

	res, err := s.Exec(ctx, `
		INSERT INTO passkey_challenges (challenge, expires_at) VALUES (?, ?)
		ON CONFLICT (challenge) DO NOTHING`, challenge, expiresAt)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

func (s *Store) scanPasskey(r interface{ Scan(...any) error }) (*pkmodel.Passkey, error) {
	var m pkmodel.Passkey
	var lastUsed sql.NullTime
	if err := r.Scan(&m.ID, &m.UserID, &m.CredentialID, &m.Name, &m.Credential, &m.CreatedAt, &lastUsed); err != nil {
		return nil, err
	}

	name, err := s.codec.OpenText(nameAAD(m.ID), m.Name)
	if err != nil {
		return nil, err
	}

	m.Name = name

	if lastUsed.Valid {
		m.LastUsedAt = &lastUsed.Time
	}

	return &m, nil
}
