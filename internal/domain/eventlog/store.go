// Gosint, Copyright (c) 2026 YouSysAdmin

package eventlog

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
)

// Store persists log entries, the message and the data sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the log store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("logs", column, id) }

func orEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}

	return m
}

// Append writes one entry.
func (s *Store) Append(ctx context.Context, e *logmodel.Entry) error {
	message, err1 := s.codec.SealText(aad("message", e.ID), e.Message)
	data, err2 := database.SealJSON(s.codec, aad("data", e.ID), orEmpty(e.Data))
	if err := errors.Join(err1, err2); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO logs (id, sketch_id, scan_id, level, message, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.SketchID, database.NullStr(e.ScanID), e.Level, message, data, e.CreatedAt)

	return err
}

// List returns the newest limit entries after since, oldest first.
func (s *Store) List(ctx context.Context, sketchID string, since *time.Time, limit int) ([]logmodel.Entry, error) {
	// A zero time stands in for no lower bound, so the query text stays
	// one constant.
	after := time.Time{}
	if since != nil {
		after = since.UTC()
	}

	rows, err := s.Query(ctx, `
		SELECT id, sketch_id, scan_id, level, message, data, created_at
		FROM logs WHERE sketch_id = ? AND created_at > ?
		ORDER BY created_at DESC, id DESC LIMIT ?`, sketchID, after, limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []logmodel.Entry
	for rows.Next() {
		var (
			e    logmodel.Entry
			data string
		)

		if err := rows.Scan(&e.ID, &e.SketchID, database.Str(&e.ScanID), &e.Level, &e.Message, &data, &e.CreatedAt); err != nil {
			return nil, err
		}

		message, err := s.codec.OpenText(aad("message", e.ID), e.Message)
		if err != nil {
			return nil, err
		}

		e.Message = message
		if err := database.OpenJSON(s.codec, aad("data", e.ID), data, &e.Data); err != nil {
			return nil, err
		}

		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	slices.Reverse(out)

	return out, nil
}

// DeleteBySketch clears the sketch's log.
func (s *Store) DeleteBySketch(ctx context.Context, sketchID string) (int64, error) {
	res, err := s.Exec(ctx, `DELETE FROM logs WHERE sketch_id = ?`, sketchID)
	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}
