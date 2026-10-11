// Gosint, Copyright (c) 2026 YouSysAdmin

package vaultkey

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"github.com/yousysadmin/gosint/internal/database"
	keymodel "github.com/yousysadmin/gosint/internal/models/vaultkey"
)

// Store persists secrets. The value is sealed under its owner, the
// name a keyed hash in name with its sealed copy in name_sealed.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the key store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

const namePurpose = "gosint/secret-name"

// ValueAAD binds a sealed secret to its owner and name, the pair its row
// is upserted on, so a secret moved to another name or account does not
// open.
func ValueAAD(codec database.Codec, ownerID, name string) (string, error) {
	idx, err := codec.Index(namePurpose, name)
	if err != nil {
		return "", err
	}

	return database.AAD("vault_keys", "value", ownerID+":"+idx), nil
}

// nameAAD binds a sealed name to its owner, not its row: an upsert on
// the name keeps the row there and takes the new value.
func nameAAD(ownerID string) string { return database.AAD("vault_keys", "name_sealed", ownerID) }

const keySelect = `
SELECT id, owner_id, name_sealed, value, created_at, updated_at
FROM vault_keys`

func (s *Store) scanKey(row interface{ Scan(...any) error }) (*keymodel.Key, error) {
	var (
		k          keymodel.Key
		nameSealed string
	)

	if err := row.Scan(&k.ID, &k.OwnerID, &nameSealed, &k.Stored, &k.CreatedAt, &k.UpdatedAt); err != nil {
		return nil, err
	}

	name, err := s.codec.OpenText(nameAAD(k.OwnerID), nameSealed)
	if err != nil {
		return nil, err
	}

	k.Name = name

	return &k, nil
}

func (s *Store) one(row *sql.Row) (*keymodel.Key, error) {
	k, err := s.scanKey(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return k, err
}

// Put stores k, replacing the owner's key of the same name. k.ID ends up
// as the stored row's id.
func (s *Store) Put(ctx context.Context, k *keymodel.Key) error {
	idx, sealed, err := s.name(k.OwnerID, k.Name)
	if err != nil {
		return err
	}

	return s.QueryRow(ctx, `
		INSERT INTO vault_keys (id, owner_id, name, name_sealed, value, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (owner_id, name) DO UPDATE SET
		    name_sealed = excluded.name_sealed, value = excluded.value, updated_at = excluded.updated_at
		RETURNING id, created_at`,
		k.ID, k.OwnerID, idx, sealed, k.Stored, k.CreatedAt, k.UpdatedAt).Scan(&k.ID, &k.CreatedAt)
}

// name is a secret name's keyed hash and sealed copy.
func (s *Store) name(ownerID, name string) (string, string, error) {
	idx, err1 := s.codec.Index(namePurpose, name)
	sealed, err2 := s.codec.SealText(nameAAD(ownerID), name)

	return idx, sealed, errors.Join(err1, err2)
}

// List lists the owner's keys by name.
func (s *Store) List(ctx context.Context, ownerID string) ([]keymodel.Key, error) {
	out, err := s.list(ctx, keySelect+` WHERE owner_id = ?`, ownerID)
	if err != nil {
		return nil, err
	}

	// By name, sorted here since the names are sealed.
	slices.SortFunc(out, func(a, b keymodel.Key) int { return strings.Compare(a.Name, b.Name) })

	return out, nil
}

func (s *Store) list(ctx context.Context, query string, args ...any) ([]keymodel.Key, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []keymodel.Key
	for rows.Next() {
		k, err := s.scanKey(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *k)
	}

	return out, rows.Err()
}

// Get returns one of the owner's keys by id.
func (s *Store) Get(ctx context.Context, ownerID, id string) (*keymodel.Key, error) {
	return s.one(s.QueryRow(ctx, keySelect+` WHERE owner_id = ? AND id = ?`, ownerID, id))
}

// GetByName returns one of the owner's keys by name.
func (s *Store) GetByName(ctx context.Context, ownerID, name string) (*keymodel.Key, error) {
	idx, err := s.codec.Index(namePurpose, name)
	if err != nil {
		return nil, err
	}

	return s.one(s.QueryRow(ctx, keySelect+` WHERE owner_id = ? AND name = ?`, ownerID, idx))
}

// Delete removes one of the owner's keys.
func (s *Store) Delete(ctx context.Context, ownerID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM vault_keys WHERE owner_id = ? AND id = ?`, ownerID, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
