// Gosint, Copyright (c) 2026 YouSysAdmin

package sketch

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yousysadmin/gosint/internal/database"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
)

// Store persists sketches, the title and the description sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the sketch store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("sketches", column, id) }

// sealed is sk's title and description as stored.
func (s *Store) sealed(sk *sketchmodel.Sketch) (string, string, error) {
	title, err1 := s.codec.SealText(aad("title", sk.ID), sk.Title)
	desc, err2 := s.codec.SealText(aad("description", sk.ID), sk.Description)

	return title, desc, errors.Join(err1, err2)
}

const sketchSelect = `
SELECT s.id, s.investigation_id, s.title, s.description, s.status, s.owner_id, s.created_at, s.updated_at
FROM sketches s`

func (s *Store) scanSketch(row interface{ Scan(...any) error }) (*sketchmodel.Sketch, error) {
	var sk sketchmodel.Sketch
	err := row.Scan(&sk.ID, &sk.InvestigationID, &sk.Title, &sk.Description, &sk.Status,
		database.Str(&sk.OwnerID), &sk.CreatedAt, &sk.UpdatedAt)
	if err != nil {
		return nil, err
	}

	title, err1 := s.codec.OpenText(aad("title", sk.ID), sk.Title)
	desc, err2 := s.codec.OpenText(aad("description", sk.ID), sk.Description)
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}

	sk.Title, sk.Description = title, desc

	return &sk, nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]sketchmodel.Sketch, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []sketchmodel.Sketch
	for rows.Next() {
		sk, err := s.scanSketch(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *sk)
	}

	return out, rows.Err()
}

// Create inserts sk.
func (s *Store) Create(ctx context.Context, sk *sketchmodel.Sketch) error {
	title, desc, err := s.sealed(sk)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO sketches (id, investigation_id, title, description, status, owner_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		sk.ID, sk.InvestigationID, title, desc, sk.Status, database.NullStr(sk.OwnerID),
		sk.CreatedAt, sk.UpdatedAt)

	return err
}

// Get returns one sketch.
func (s *Store) Get(ctx context.Context, id string) (*sketchmodel.Sketch, error) {
	sk, err := s.scanSketch(s.QueryRow(ctx, sketchSelect+` WHERE s.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return sk, err
}

// ListByInvestigation lists an investigation's sketches, newest change first.
func (s *Store) ListByInvestigation(ctx context.Context, investigationID string) ([]sketchmodel.Sketch, error) {
	return s.list(ctx, sketchSelect+`
		WHERE s.investigation_id = ?
		ORDER BY s.updated_at DESC, s.id`, investigationID)
}

// ListForUser lists the sketches of userID's investigations.
func (s *Store) ListForUser(ctx context.Context, userID string) ([]sketchmodel.Sketch, error) {
	return s.list(ctx, sketchSelect+`
		JOIN investigation_members m ON m.investigation_id = s.investigation_id
		WHERE m.user_id = ?
		ORDER BY s.updated_at DESC, s.id`, userID)
}

// Update writes the editable fields.
func (s *Store) Update(ctx context.Context, sk *sketchmodel.Sketch) error {
	title, desc, err := s.sealed(sk)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE sketches SET title = ?, description = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		title, desc, sk.Status, sk.UpdatedAt, sk.ID)

	return err
}

// Delete removes the sketch and, by cascade, its graph.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.Exec(ctx, `DELETE FROM sketches WHERE id = ?`, id)

	return err
}
