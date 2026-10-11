// Gosint, Copyright (c) 2026 YouSysAdmin

package scan

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// Store persists scans, the job queue. What a scan ran on, what it
// found and its error are sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the scan store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("scans", column, id) }

const scanSelect = `
SELECT id, sketch_id, owner_id, kind, name, status, input, details, error, attempts,
       claimed_by, lease_until, created_at, started_at, completed_at
FROM scans`

func (s *Store) scanRow(row interface{ Scan(...any) error }) (*scanmodel.Scan, error) {
	var (
		sc                        scanmodel.Scan
		input, details            string
		lease, started, completed sql.NullTime
	)

	err := row.Scan(&sc.ID, database.Str(&sc.SketchID), database.Str(&sc.OwnerID), &sc.Kind, &sc.Name, &sc.Status,
		&input, &details, &sc.Error, &sc.Attempts, &sc.ClaimedBy, &lease, &sc.CreatedAt, &started, &completed)
	if err != nil {
		return nil, err
	}

	if sc.Name, err = s.codec.OpenText(aad("name", sc.ID), sc.Name); err != nil {
		return nil, err
	}

	if err := database.OpenJSON(s.codec, aad("input", sc.ID), input, &sc.Input); err != nil {
		return nil, err
	}

	if err := database.OpenJSON(s.codec, aad("details", sc.ID), details, &sc.Details); err != nil {
		return nil, err
	}

	if sc.Error, err = s.codec.OpenText(aad("error", sc.ID), sc.Error); err != nil {
		return nil, err
	}

	sc.LeaseUntil = timePtr(lease)
	sc.StartedAt = timePtr(started)
	sc.CompletedAt = timePtr(completed)

	return &sc, nil
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}

	return new(t.Time.UTC())
}

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}

	return m
}

// Enqueue inserts a pending scan.
func (s *Store) Enqueue(ctx context.Context, sc *scanmodel.Scan) error {
	input, err1 := database.SealJSON(s.codec, aad("input", sc.ID), orEmpty(sc.Input))
	details, err2 := database.SealJSON(s.codec, aad("details", sc.ID), orEmpty(sc.Details))
	name, err3 := s.codec.SealText(aad("name", sc.ID), sc.Name)
	if err := errors.Join(err1, err2, err3); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO scans (id, sketch_id, owner_id, kind, name, status, input, details, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.ID, database.NullStr(sc.SketchID), database.NullStr(sc.OwnerID), sc.Kind, name, sc.Status,
		input, details, sc.CreatedAt)

	return err
}

// Get returns one scan.
func (s *Store) Get(ctx context.Context, id string) (*scanmodel.Scan, error) {
	sc, err := s.scanRow(s.QueryRow(ctx, scanSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return sc, err
}

// ListBySketch lists the sketch's newest scans first.
func (s *Store) ListBySketch(ctx context.Context, sketchID string, limit int) ([]scanmodel.Scan, error) {
	return s.list(ctx, scanSelect+` WHERE sketch_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, sketchID, limit)
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]scanmodel.Scan, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []scanmodel.Scan
	for rows.Next() {
		sc, err := s.scanRow(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *sc)
	}

	return out, rows.Err()
}

// Delete removes a scan. Its log lines stay, detached from it.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.Exec(ctx, `DELETE FROM scans WHERE id = ?`, id)

	return err
}

// Claim takes up to limit pending scans, oldest first.
//
// Two steps rather than FOR UPDATE SKIP LOCKED, which sqlite has not
// got: read candidates, then take each with an UPDATE that only matches
// while the row is still pending. A worker that loses the race on a row
// simply does not get it.
func (s *Store) Claim(ctx context.Context, worker string, now, lease time.Time, limit int) ([]scanmodel.Scan, error) {
	if limit <= 0 {
		return nil, nil
	}

	candidates, err := s.list(ctx, scanSelect+` WHERE status = ? ORDER BY created_at, id LIMIT ?`,
		scanmodel.StatusPending, limit)
	if err != nil {
		return nil, err
	}

	var out []scanmodel.Scan
	for _, c := range candidates {
		res, err := s.Exec(ctx, `
			UPDATE scans SET status = ?, claimed_by = ?, lease_until = ?, started_at = ?, attempts = attempts + 1
			WHERE id = ? AND status = ?`,
			scanmodel.StatusRunning, worker, lease, now, c.ID, scanmodel.StatusPending)
		if err != nil {
			return out, err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return out, err
		}

		if n == 0 {
			continue
		}

		c.Status = scanmodel.StatusRunning
		c.ClaimedBy = worker
		c.LeaseUntil = new(lease)
		c.StartedAt = new(now)
		c.Attempts++
		out = append(out, c)
	}

	return out, nil
}

// Renew extends a running scan's lease for the worker holding it.
func (s *Store) Renew(ctx context.Context, id, worker string, lease time.Time) (bool, error) {
	res, err := s.Exec(ctx, `UPDATE scans SET lease_until = ? WHERE id = ? AND claimed_by = ? AND status = ?`,
		lease, id, worker, scanmodel.StatusRunning)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Finish stores the outcome of a running scan held by worker.
func (s *Store) Finish(ctx context.Context, sc *scanmodel.Scan, worker string) (bool, error) {
	details, err1 := database.SealJSON(s.codec, aad("details", sc.ID), orEmpty(sc.Details))
	failure, err2 := s.codec.SealText(aad("error", sc.ID), sc.Error)
	if err := errors.Join(err1, err2); err != nil {
		return false, err
	}

	res, err := s.Exec(ctx, `
		UPDATE scans SET status = ?, details = ?, error = ?, completed_at = ?, lease_until = NULL
		WHERE id = ? AND claimed_by = ? AND status = ?`,
		sc.Status, details, failure, database.NullTime(sc.CompletedAt),
		sc.ID, worker, scanmodel.StatusRunning)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Release puts a running scan held by worker back to pending.
func (s *Store) Release(ctx context.Context, id, worker string) (bool, error) {
	res, err := s.Exec(ctx, `
		UPDATE scans SET status = ?, claimed_by = '', lease_until = NULL
		WHERE id = ? AND claimed_by = ? AND status = ?`,
		scanmodel.StatusPending, id, worker, scanmodel.StatusRunning)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Cancel stops a pending or running scan. A running one notices on its
// next lease renewal.
func (s *Store) Cancel(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.Exec(ctx, `
		UPDATE scans SET status = ?, completed_at = ?, lease_until = NULL
		WHERE id = ? AND status IN (?, ?)`,
		scanmodel.StatusCancelled, now, id, scanmodel.StatusPending, scanmodel.StatusRunning)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// RecoverExpired requeues running scans whose worker stopped renewing,
// or fails them once they reached maxAttempts.
func (s *Store) RecoverExpired(ctx context.Context, now time.Time, maxAttempts int) (int64, int64, error) {
	// The error is sealed under each scan, so the ones to fail are read
	// first and each written only while it is still expired.
	dead, err := s.expired(ctx, now, maxAttempts)
	if err != nil {
		return 0, 0, err
	}

	var f int64
	for _, id := range dead {
		reason, err := s.codec.SealText(aad("error", id), "the worker running this scan stopped responding")
		if err != nil {
			return 0, 0, err
		}

		res, err := s.Exec(ctx, `
			UPDATE scans SET status = ?, error = ?, completed_at = ?, lease_until = NULL
			WHERE id = ? AND status = ? AND lease_until < ?`,
			scanmodel.StatusFailed, reason, now, id, scanmodel.StatusRunning, now)
		if err != nil {
			return 0, 0, err
		}

		n, err := res.RowsAffected()
		if err != nil {
			return 0, 0, err
		}

		f += n
	}

	requeued, err := s.Exec(ctx, `
		UPDATE scans SET status = ?, claimed_by = '', lease_until = NULL
		WHERE status = ? AND lease_until < ?`,
		scanmodel.StatusPending, scanmodel.StatusRunning, now)
	if err != nil {
		return 0, 0, err
	}

	r, err := requeued.RowsAffected()

	return r, f, err
}

// expired is the running scans whose lease ran out at their last attempt.
func (s *Store) expired(ctx context.Context, now time.Time, maxAttempts int) ([]string, error) {
	rows, err := s.Query(ctx, `SELECT id FROM scans WHERE status = ? AND lease_until < ? AND attempts >= ?`,
		scanmodel.StatusRunning, now, maxAttempts)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}

		out = append(out, id)
	}

	return out, rows.Err()
}
