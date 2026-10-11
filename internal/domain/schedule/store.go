// Gosint, Copyright (c) 2026 YouSysAdmin

// Package schedule runs flows again on a sketch every so often.
package schedule

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	smodel "github.com/yousysadmin/gosint/internal/models/schedule"
)

// Store persists the schedules, the flow's name, the entities it runs
// on and the last error sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("schedules", column, id) }

const sel = `
SELECT id, sketch_id, owner_id, flow_id, flow_name, node_ids, every_seconds, enabled, version,
       last_run_at, last_scan_id, prev_scan_id, last_error, created_at, updated_at
FROM schedules`

func (s *Store) scanOne(row interface{ Scan(...any) error }) (*smodel.Schedule, error) {
	var (
		sc    smodel.Schedule
		nodes string
	)
	err := row.Scan(&sc.ID, &sc.SketchID, &sc.OwnerID, &sc.FlowID, &sc.FlowName, &nodes, &sc.Every, &sc.Enabled, &sc.Version,
		&sc.LastRunAt, &sc.LastScanID, &sc.PrevScanID, &sc.LastError, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}

	var errs [3]error
	sc.FlowName, errs[0] = s.codec.OpenText(aad("flow_name", sc.ID), sc.FlowName)
	sc.LastError, errs[1] = s.codec.OpenText(aad("last_error", sc.ID), sc.LastError)
	sc.NodeIDs = []string{}
	errs[2] = database.OpenJSON(s.codec, aad("node_ids", sc.ID), nodes, &sc.NodeIDs)

	return &sc, errors.Join(errs[:]...)
}

func (s *Store) collect(rows *sql.Rows, err error) ([]smodel.Schedule, error) {
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []smodel.Schedule{}
	for rows.Next() {
		sc, err := s.scanOne(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *sc)
	}

	return out, rows.Err()
}

// Create inserts s.
func (s *Store) Create(ctx context.Context, sc *smodel.Schedule) error {
	name, err1 := s.codec.SealText(aad("flow_name", sc.ID), sc.FlowName)
	nodes, err2 := database.SealJSON(s.codec, aad("node_ids", sc.ID), sc.NodeIDs)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO schedules (id, sketch_id, owner_id, flow_id, flow_name, node_ids, every_seconds, enabled, version,
		                       last_run_at, last_scan_id, prev_scan_id, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, NULL, '', '', '', ?, ?)`,
		sc.ID, sc.SketchID, sc.OwnerID, sc.FlowID, name, nodes, sc.Every, sc.Enabled,
		sc.CreatedAt, sc.UpdatedAt)

	return err
}

// Get is one schedule of the sketch, nil when absent.
func (s *Store) Get(ctx context.Context, sketchID, id string) (*smodel.Schedule, error) {
	sc, err := s.scanOne(s.QueryRow(ctx, sel+` WHERE sketch_id = ? AND id = ?`, sketchID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return sc, err
}

// ListBySketch lists the sketch's schedules, oldest first.
func (s *Store) ListBySketch(ctx context.Context, sketchID string) ([]smodel.Schedule, error) {
	return s.collect(s.Query(ctx, sel+` WHERE sketch_id = ? ORDER BY created_at, id`, sketchID))
}

// ListEnabled lists every enabled schedule, for the tick.
func (s *Store) ListEnabled(ctx context.Context) ([]smodel.Schedule, error) {
	return s.collect(s.Query(ctx, sel+` WHERE enabled ORDER BY last_run_at, id`))
}

// Update writes the settings back: the interval, the selection, the
// switch.
func (s *Store) Update(ctx context.Context, sc *smodel.Schedule) error {
	nodes, err := database.SealJSON(s.codec, aad("node_ids", sc.ID), sc.NodeIDs)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `UPDATE schedules SET node_ids = ?, every_seconds = ?, enabled = ?, updated_at = ? WHERE id = ? AND sketch_id = ?`,
		nodes, sc.Every, sc.Enabled, sc.UpdatedAt, sc.ID, sc.SketchID)

	return err
}

// Delete removes one schedule.
func (s *Store) Delete(ctx context.Context, sketchID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM schedules WHERE id = ? AND sketch_id = ?`, id, sketchID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// Claim records a run: the scan queued, now as the last run, the
// previous scan kept for the diff. It takes the version the schedule
// was read at and reports false when another node ran it first.
func (s *Store) Claim(ctx context.Context, sc *smodel.Schedule, scanID string, now time.Time) (bool, error) {
	res, err := s.Exec(ctx, `
		UPDATE schedules SET version = version + 1, last_run_at = ?, prev_scan_id = last_scan_id, last_scan_id = ?,
		       last_error = '', updated_at = ?
		WHERE id = ? AND version = ?`,
		now, scanID, now, sc.ID, sc.Version)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()
	if n > 0 {
		sc.Version++
		sc.PrevScanID, sc.LastScanID, sc.LastRunAt = sc.LastScanID, scanID, &now
	}

	return n > 0, err
}

// Fail notes why a run could not be queued and switches the schedule
// off, so a flow that is gone does not fail every minute.
func (s *Store) Fail(ctx context.Context, id, reason string, now time.Time) error {
	sealed, err := s.codec.SealText(aad("last_error", id), reason)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `UPDATE schedules SET enabled = ?, last_error = ?, updated_at = ? WHERE id = ?`, false, sealed, now, id)

	return err
}
