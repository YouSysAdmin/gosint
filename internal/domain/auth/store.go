// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	sessionmodel "github.com/yousysadmin/gosint/internal/models/session"
)

// SessionStore persists sessions, the browser and the address sealed.
type SessionStore struct {
	database.Base
	codec database.Codec
}

// NewSessionStore binds the session store to db.
func NewSessionStore(db database.Database, codec database.Codec) *SessionStore {
	return &SessionStore{Base: database.NewBase(db), codec: codec}
}

func sessionAAD(column, id string) string { return database.AAD("sessions", column, id) }

// open opens the sealed columns of a session read back.
func (s *SessionStore) open(sess *sessionmodel.Session) error {
	agent, err1 := s.codec.OpenText(sessionAAD("user_agent", sess.ID), sess.UserAgent)
	ip, err2 := s.codec.OpenText(sessionAAD("client_ip", sess.ID), sess.ClientIP)
	sess.UserAgent, sess.ClientIP = agent, ip

	return errors.Join(err1, err2)
}

// Create inserts s.
func (s *SessionStore) Create(ctx context.Context, sess *sessionmodel.Session) error {
	agent, err1 := s.codec.SealText(sessionAAD("user_agent", sess.ID), sess.UserAgent)
	ip, err2 := s.codec.SealText(sessionAAD("client_ip", sess.ID), sess.ClientIP)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, user_agent, client_ip, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.TokenHash, agent, ip, sess.CreatedAt, sess.ExpiresAt)

	return err
}

// Lookup returns the unexpired session for tokenHash.
func (s *SessionStore) Lookup(ctx context.Context, tokenHash string, now time.Time) (*sessionmodel.Session, error) {
	var sess sessionmodel.Session
	err := s.QueryRow(ctx, `
		SELECT id, user_id, token_hash, user_agent, client_ip, created_at, expires_at
		FROM sessions WHERE token_hash = ? AND expires_at > ?`, tokenHash, now).
		Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.UserAgent, &sess.ClientIP,
			&sess.CreatedAt, &sess.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &sess, s.open(&sess)
}

// Extend moves a session's expiry.
func (s *SessionStore) Extend(ctx context.Context, id string, expiresAt time.Time) error {
	_, err := s.Exec(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`, expiresAt, id)

	return err
}

// Delete ends the session for tokenHash.
func (s *SessionStore) Delete(ctx context.Context, tokenHash string) error {
	_, err := s.Exec(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)

	return err
}

// ListForUser lists the account's live sessions, newest first.
func (s *SessionStore) ListForUser(ctx context.Context, userID string, now time.Time) ([]sessionmodel.Session, error) {
	rows, err := s.Query(ctx, `
		SELECT id, user_id, token_hash, user_agent, client_ip, created_at, expires_at
		FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC`, userID, now)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []sessionmodel.Session{}
	for rows.Next() {
		var sess sessionmodel.Session
		if err := rows.Scan(&sess.ID, &sess.UserID, &sess.TokenHash, &sess.UserAgent, &sess.ClientIP,
			&sess.CreatedAt, &sess.ExpiresAt); err != nil {
			return nil, err
		}

		if err := s.open(&sess); err != nil {
			return nil, err
		}

		out = append(out, sess)
	}

	return out, rows.Err()
}

// DeleteByID ends one of the account's sessions, false when it had none
// by that id.
func (s *SessionStore) DeleteByID(ctx context.Context, userID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM sessions WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// DeleteOthers ends every session of the account but keepID.
func (s *SessionStore) DeleteOthers(ctx context.Context, userID, keepID string) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepID)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// DeleteForUser ends every session of the account.
func (s *SessionStore) DeleteForUser(ctx context.Context, userID string) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

// DeleteExpired sweeps sessions that ended before now.
func (s *SessionStore) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
