// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/yousysadmin/gosint/internal/database"
	tplmodel "github.com/yousysadmin/gosint/internal/models/transformtemplate"
)

// Store persists transform templates. The name is a keyed hash in name
// with its sealed copy, everything else the person wrote sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the template store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

const namePurpose = "gosint/template-name"

func aad(column, id string) string { return database.AAD("transform_templates", column, id) }

const templateSelect = `
SELECT id, owner_id, name_sealed, description, category, input_type, output_type, source, is_public, created_at, updated_at
FROM transform_templates`

// sealedFields are the columns sealed in place, by their model field.
func sealedFields(t *tplmodel.Template) []struct {
	col string
	v   *string
} {
	return []struct {
		col string
		v   *string
	}{
		{"description", &t.Description}, {"category", &t.Category}, {"input_type", &t.InputType},
		{"output_type", &t.OutputType}, {"source", &t.Source},
	}
}

func (s *Store) scanTemplate(row interface{ Scan(...any) error }) (*tplmodel.Template, error) {
	var (
		t          tplmodel.Template
		nameSealed string
	)

	err := row.Scan(&t.ID, &t.OwnerID, &nameSealed, &t.Description, &t.Category, &t.InputType, &t.OutputType,
		&t.Source, &t.Public, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	name, err := s.codec.OpenText(aad("name_sealed", t.ID), nameSealed)
	errs := []error{err}
	t.Name = name

	for _, f := range sealedFields(&t) {
		v, err := s.codec.OpenText(aad(f.col, t.ID), *f.v)
		errs = append(errs, err)
		*f.v = v
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &t, nil
}

// columns is t's name index, sealed name, then the sealed fields in
// sealedFields order.
func (s *Store) columns(t *tplmodel.Template) ([]any, error) {
	idx, err := s.codec.Index(namePurpose, t.Name)
	if err != nil {
		return nil, err
	}

	name, err := s.codec.SealText(aad("name_sealed", t.ID), t.Name)
	if err != nil {
		return nil, err
	}

	out := []any{idx, name}
	for _, f := range sealedFields(t) {
		v, err := s.codec.SealText(aad(f.col, t.ID), *f.v)
		if err != nil {
			return nil, err
		}

		out = append(out, v)
	}

	return out, nil
}

// Create inserts t. A second template of the same name for one owner is
// a unique violation.
func (s *Store) Create(ctx context.Context, t *tplmodel.Template) error {
	c, err := s.columns(t)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO transform_templates (id, owner_id, name, name_sealed, description, category, input_type, output_type,
		                                source, is_public, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append(append([]any{t.ID, t.OwnerID}, c...), t.Public, t.CreatedAt, t.UpdatedAt)...)

	return err
}

// Update rewrites one of the owner's templates.
func (s *Store) Update(ctx context.Context, t *tplmodel.Template) error {
	c, err := s.columns(t)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE transform_templates SET name = ?, name_sealed = ?, description = ?, category = ?, input_type = ?,
		       output_type = ?, source = ?, is_public = ?, updated_at = ?
		WHERE id = ? AND owner_id = ?`,
		append(c, t.Public, t.UpdatedAt, t.ID, t.OwnerID)...)

	return err
}

// Get returns a template userID may see.
func (s *Store) Get(ctx context.Context, userID, id string) (*tplmodel.Template, error) {
	t, err := s.scanTemplate(s.QueryRow(ctx, templateSelect+` WHERE id = ? AND (owner_id = ? OR is_public = ?)`,
		id, userID, true))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return t, err
}

// Visible lists userID's templates and the public ones, by name, sorted
// here since the names are sealed.
func (s *Store) Visible(ctx context.Context, userID string) ([]tplmodel.Template, error) {
	out, err := s.list(ctx, templateSelect+` WHERE owner_id = ? OR is_public = ?`, userID, true)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b tplmodel.Template) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })

	return out, nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]tplmodel.Template, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []tplmodel.Template
	for rows.Next() {
		t, err := s.scanTemplate(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *t)
	}

	return out, rows.Err()
}

// Delete removes one of the owner's templates.
func (s *Store) Delete(ctx context.Context, ownerID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM transform_templates WHERE id = ? AND owner_id = ?`, id, ownerID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
