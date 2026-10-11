// Gosint, Copyright (c) 2026 YouSysAdmin

// Package llmmodel is the language models an administrator offers:
// storage, the admin endpoints and the list a person picks from.
package llmmodel

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	model "github.com/yousysadmin/gosint/internal/models/llmmodel"
)

// Store persists the models, each sealed whole as JSON but its default
// switch.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db and the vault.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(id string) string { return database.AAD("llm_models", "data", id) }

const sel = `SELECT id, data, is_default, created_at, updated_at FROM llm_models`

func (s *Store) scan(row interface{ Scan(...any) error }) (*model.Model, error) {
	var (
		m    model.Model
		data string
	)

	if err := row.Scan(&m.ID, &data, &m.Default, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}

	id, def, created, updated := m.ID, m.Default, m.CreatedAt, m.UpdatedAt
	if err := database.OpenJSON(s.codec, aad(id), data, &m); err != nil {
		return nil, err
	}

	m.ID, m.Default, m.CreatedAt, m.UpdatedAt = id, def, created, updated

	return &m, nil
}

// List is every model, by name, sorted here since the names are sealed.
func (s *Store) List(ctx context.Context) ([]model.Model, error) {
	rows, err := s.Query(ctx, sel)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []model.Model{}
	for rows.Next() {
		m, err := s.scan(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b model.Model) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })

	return out, nil
}

// Get is one model, nil when absent.
func (s *Store) Get(ctx context.Context, id string) (*model.Model, error) {
	m, err := s.scan(s.QueryRow(ctx, sel+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return m, err
}

// Put inserts or replaces m by id. A default model takes the switch
// from every other one in the same transaction.
func (s *Store) Put(ctx context.Context, m *model.Model) error {
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}

	m.UpdatedAt = now
	data, err := database.SealJSON(s.codec, aad(m.ID), m)
	if err != nil {
		return err
	}

	return s.Tx(ctx, func(tx *database.Tx) error {
		if m.Default {
			if _, err := tx.Exec(ctx, `UPDATE llm_models SET is_default = ? WHERE id <> ?`, false, m.ID); err != nil {
				return err
			}
		}

		_, err := tx.Exec(ctx, `
			INSERT INTO llm_models (id, data, is_default, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET data = excluded.data, is_default = excluded.is_default, updated_at = excluded.updated_at`,
			m.ID, data, m.Default, m.CreatedAt, m.UpdatedAt)

		return err
	})
}

// Delete removes one model.
func (s *Store) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM llm_models WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
