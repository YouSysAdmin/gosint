// Gosint, Copyright (c) 2026 YouSysAdmin

package analysis

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yousysadmin/gosint/internal/database"
	amodel "github.com/yousysadmin/gosint/internal/models/analysis"
)

// Store persists analyses, the title and the document sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the analysis store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("analyses", column, id) }

// sealed is a's title and content as stored.
func (s *Store) sealed(a *amodel.Analysis) (string, string, error) {
	title, err1 := s.codec.SealText(aad("title", a.ID), a.Title)
	content, err2 := s.codec.SealText(aad("content", a.ID), string(a.Content))

	return title, content, errors.Join(err1, err2)
}

const analysisSelect = `
SELECT id, investigation_id, owner_id, title, content, created_at, updated_at FROM analyses`

func (s *Store) scanAnalysis(row interface{ Scan(...any) error }) (*amodel.Analysis, error) {
	var (
		a       amodel.Analysis
		content string
	)

	if err := row.Scan(&a.ID, &a.InvestigationID, &a.OwnerID, &a.Title, &content, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}

	title, err1 := s.codec.OpenText(aad("title", a.ID), a.Title)
	content, err2 := s.codec.OpenText(aad("content", a.ID), content)
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}

	// The list leaves the document out, an empty object stands for it.
	if content == "" {
		content = "{}"
	}

	a.Title, a.Content = title, []byte(content)

	return &a, nil
}

// Create inserts a.
func (s *Store) Create(ctx context.Context, a *amodel.Analysis) error {
	title, content, err := s.sealed(a)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO analyses (id, investigation_id, owner_id, title, content, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.InvestigationID, a.OwnerID, title, content, a.CreatedAt, a.UpdatedAt)

	return err
}

// Update rewrites title and content.
func (s *Store) Update(ctx context.Context, a *amodel.Analysis) error {
	title, content, err := s.sealed(a)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `UPDATE analyses SET title = ?, content = ?, updated_at = ? WHERE id = ? AND investigation_id = ?`,
		title, content, a.UpdatedAt, a.ID, a.InvestigationID)

	return err
}

// Get returns one analysis of the investigation, nil when absent.
func (s *Store) Get(ctx context.Context, investigationID, id string) (*amodel.Analysis, error) {
	a, err := s.scanAnalysis(s.QueryRow(ctx, analysisSelect+` WHERE id = ? AND investigation_id = ?`, id, investigationID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return a, err
}

// List lists the investigation's analyses, latest first, without their
// content.
func (s *Store) List(ctx context.Context, investigationID string) ([]amodel.Analysis, error) {
	rows, err := s.Query(ctx, `
		SELECT id, investigation_id, owner_id, title, '', created_at, updated_at FROM analyses
		WHERE investigation_id = ? ORDER BY updated_at DESC, id DESC`, investigationID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []amodel.Analysis
	for rows.Next() {
		a, err := s.scanAnalysis(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *a)
	}

	return out, rows.Err()
}

// Delete removes one analysis, false when there was none.
func (s *Store) Delete(ctx context.Context, investigationID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM analyses WHERE id = ? AND investigation_id = ?`, id, investigationID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
