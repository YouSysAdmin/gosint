// Gosint, Copyright (c) 2026 YouSysAdmin

package flow

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/yousysadmin/gosint/internal/database"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Store persists flows. The name is a keyed hash in name with its
// sealed copy, the input type a keyed hash, the description and the
// definition sealed: what a person chains says what they work on.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the flow store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

// Index purposes. The input type shares the node type's, both name an
// entity type.
const (
	namePurpose = "gosint/flow-name"
	typePurpose = graphmodel.TypePurpose
)

func aad(column, id string) string { return database.AAD("flows", column, id) }

const flowSelect = `
SELECT id, owner_id, name_sealed, description, definition, created_at, updated_at
FROM flows`

func (s *Store) scanFlow(row interface{ Scan(...any) error }) (*flowmodel.Flow, error) {
	var (
		f                flowmodel.Flow
		nameSealed, def  string
		desc             string
		err1, err2, err3 error
	)

	if err := row.Scan(&f.ID, &f.OwnerID, &nameSealed, &desc, &def, &f.CreatedAt, &f.UpdatedAt); err != nil {
		return nil, err
	}

	f.Name, err1 = s.codec.OpenText(aad("name_sealed", f.ID), nameSealed)

	f.Description, err2 = s.codec.OpenText(aad("description", f.ID), desc)
	err3 = database.OpenJSON(s.codec, aad("definition", f.ID), def, &f.Definition)
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, err
	}

	return &f, nil
}

// columns is f's name index, sealed name, input type index, sealed
// description and sealed definition.
func (s *Store) columns(f *flowmodel.Flow) ([5]string, error) {
	var (
		c    [5]string
		errs [5]error
	)

	c[0], errs[0] = s.codec.Index(namePurpose, f.Name)
	c[1], errs[1] = s.codec.SealText(aad("name_sealed", f.ID), f.Name)
	c[2], errs[2] = s.codec.Index(typePurpose, f.Definition.Input)
	c[3], errs[3] = s.codec.SealText(aad("description", f.ID), f.Description)
	c[4], errs[4] = database.SealJSON(s.codec, aad("definition", f.ID), f.Definition)

	return c, errors.Join(errs[:]...)
}

// Create inserts f.
func (s *Store) Create(ctx context.Context, f *flowmodel.Flow) error {
	c, err := s.columns(f)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO flows (id, owner_id, name, name_sealed, input_type, description, definition, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.OwnerID, c[0], c[1], c[2], c[3], c[4], f.CreatedAt, f.UpdatedAt)

	return err
}

// Update rewrites one of the owner's flows.
func (s *Store) Update(ctx context.Context, f *flowmodel.Flow) error {
	c, err := s.columns(f)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE flows SET name = ?, name_sealed = ?, input_type = ?, description = ?, definition = ?, updated_at = ?
		WHERE id = ? AND owner_id = ?`,
		c[0], c[1], c[2], c[3], c[4], f.UpdatedAt, f.ID, f.OwnerID)

	return err
}

// Get returns one of the owner's flows.
func (s *Store) Get(ctx context.Context, ownerID, id string) (*flowmodel.Flow, error) {
	f, err := s.scanFlow(s.QueryRow(ctx, flowSelect+` WHERE id = ? AND owner_id = ?`, id, ownerID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return f, err
}

// List lists the owner's flows by name, optionally by input type,
// matched by its keyed hash. Sorted here since the names are sealed.
func (s *Store) List(ctx context.Context, ownerID, inputType string) ([]flowmodel.Flow, error) {
	query, args := flowSelect+` WHERE owner_id = ?`, []any{ownerID}
	if inputType != "" {
		idx, err := s.codec.Index(typePurpose, inputType)
		if err != nil {
			return nil, err
		}

		query += ` AND input_type = ?`
		args = append(args, idx)
	}

	out, err := s.list(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(out, func(a, b flowmodel.Flow) int { return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID)) })

	return out, nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]flowmodel.Flow, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []flowmodel.Flow
	for rows.Next() {
		f, err := s.scanFlow(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *f)
	}

	return out, rows.Err()
}

// Delete removes one of the owner's flows.
func (s *Store) Delete(ctx context.Context, ownerID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM flows WHERE id = ? AND owner_id = ?`, id, ownerID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
