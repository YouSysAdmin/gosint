// Gosint, Copyright (c) 2026 YouSysAdmin

// Package customtype keeps the entity types an administrator declares
// and lays them over the type registry.
package customtype

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database"
	ctmodel "github.com/yousysadmin/gosint/internal/models/customtype"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Store persists the declarations. The name is a keyed hash, the one
// entity types are stored by, with its sealed copy, the title,
// description, category and fields sealed, the icon and colour as they
// are.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(column, id string) string { return database.AAD("custom_types", column, id) }

const sel = `SELECT id, name_sealed, title, description, category, icon, color, fields, created_by, created_at, updated_at FROM custom_types`

func (s *Store) scanOne(row interface{ Scan(...any) error }) (*ctmodel.CustomType, error) {
	var (
		t                  ctmodel.CustomType
		nameSealed, fields string
	)
	err := row.Scan(&t.ID, &nameSealed, &t.Title, &t.Description, &t.Category, &t.Icon, &t.Color, &fields, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}

	var errs [5]error
	t.Name, errs[0] = s.codec.OpenText(aad("name_sealed", t.ID), nameSealed)

	t.Title, errs[1] = s.codec.OpenText(aad("title", t.ID), t.Title)
	t.Description, errs[2] = s.codec.OpenText(aad("description", t.ID), t.Description)
	t.Category, errs[3] = s.codec.OpenText(aad("category", t.ID), t.Category)
	t.Fields = []types.CustomField{}
	errs[4] = database.OpenJSON(s.codec, aad("fields", t.ID), fields, &t.Fields)

	return &t, errors.Join(errs[:]...)
}

// columns is t's name index, sealed name, title, description, category
// and fields.
func (s *Store) columns(t *ctmodel.CustomType) ([6]string, error) {
	var (
		c    [6]string
		errs [6]error
	)

	c[0], errs[0] = s.codec.Index(graphmodel.TypePurpose, t.Name)
	c[1], errs[1] = s.codec.SealText(aad("name_sealed", t.ID), t.Name)
	c[2], errs[2] = s.codec.SealText(aad("title", t.ID), t.Title)
	c[3], errs[3] = s.codec.SealText(aad("description", t.ID), t.Description)
	c[4], errs[4] = s.codec.SealText(aad("category", t.ID), t.Category)
	c[5], errs[5] = database.SealJSON(s.codec, aad("fields", t.ID), t.Fields)

	return c, errors.Join(errs[:]...)
}

// List is every declaration by name, sorted here since the names are
// sealed.
func (s *Store) List(ctx context.Context) ([]ctmodel.CustomType, error) {
	out, err := s.list(ctx, sel)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b ctmodel.CustomType) int { return strings.Compare(a.Name, b.Name) })

	return out, nil
}

func (s *Store) list(ctx context.Context, query string) ([]ctmodel.CustomType, error) {
	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []ctmodel.CustomType{}
	for rows.Next() {
		t, err := s.scanOne(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *t)
	}

	return out, rows.Err()
}

// Get is one declaration, nil when absent.
func (s *Store) Get(ctx context.Context, id string) (*ctmodel.CustomType, error) {
	t, err := s.scanOne(s.QueryRow(ctx, sel+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return t, err
}

// Create inserts t.
func (s *Store) Create(ctx context.Context, t *ctmodel.CustomType) error {
	c, err := s.columns(t)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO custom_types (id, name, name_sealed, title, description, category, icon, color, fields, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, c[0], c[1], c[2], c[3], c[4], t.Icon, t.Color, c[5], t.CreatedBy, t.CreatedAt, t.UpdatedAt)

	return err
}

// Update writes everything but the name back.
func (s *Store) Update(ctx context.Context, t *ctmodel.CustomType) error {
	c, err := s.columns(t)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE custom_types SET title = ?, description = ?, category = ?, icon = ?, color = ?, fields = ?, updated_at = ?
		WHERE id = ?`,
		c[2], c[3], c[4], t.Icon, t.Color, c[5], t.UpdatedAt, t.ID)

	return err
}

// Delete removes one declaration.
func (s *Store) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM custom_types WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// CountNodes is how many entities of the type exist, in every sketch.
// A node's type is its keyed hash.
func (s *Store) CountNodes(ctx context.Context, typeName string) (int, error) {
	idx, err := s.codec.Index(graphmodel.TypePurpose, typeName)
	if err != nil {
		return 0, err
	}

	var n int
	err = s.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE node_type = ?`, idx).Scan(&n)

	return n, err
}
