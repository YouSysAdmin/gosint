// Gosint, Copyright (c) 2026 YouSysAdmin

// Package setting is the install-wide settings an administrator edits
// in the console: the mail relay, the worker, the tools and the file
// store.
// Each is one row of sealed JSON.
package setting

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
)

// Setting names.
const (
	Mail   = "mail"
	Worker = "worker"
	Tools  = "tools"
	Files  = "files"
)

// Store persists the settings.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db and the vault.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func aad(name string) string { return database.AAD("settings", "value", name) }

// Get reads the setting into dst, false when it was never saved.
func (s *Store) Get(ctx context.Context, name string, dst any) (bool, error) {
	var stored string
	err := s.QueryRow(ctx, `SELECT value FROM settings WHERE name = ?`, name).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return true, database.OpenJSON(s.codec, aad(name), stored, dst)
}

// Put writes the setting, by is the account saving it.
func (s *Store) Put(ctx context.Context, name string, v any, by string) error {
	sealed, err := database.SealJSON(s.codec, aad(name), v)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO settings (name, value, updated_by, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (name) DO UPDATE SET value = excluded.value, updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		name, sealed, by, time.Now().UTC())

	return err
}
