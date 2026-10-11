// Gosint, Copyright (c) 2026 YouSysAdmin

// Package db keeps blobs in the database, in the blobs table. For a
// postgres install of several nodes with no bucket to share: every
// node reaches the database already. It costs database size and a
// full read per range, so it fits screenshots and documents, not
// video.
package db

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/database"
)

// Store is the table implementation.
type Store struct {
	database.Base
	spool string
}

// New binds the store to db. Uploads are hashed through a spool file in
// spoolDir, the system temp dir when empty.
func New(d database.Database, spoolDir string) *Store {
	if spoolDir == "" {
		spoolDir = os.TempDir()
	}

	return &Store{Base: database.NewBase(d), spool: spoolDir}
}

// Put hashes the upload, then inserts the row unless one exists.
func (s *Store) Put(ctx context.Context, r io.Reader, limit int64) (string, int64, error) {
	return s.put(ctx, "", r, limit)
}

// PutNamed is Put under a name the caller chose.
func (s *Store) PutNamed(ctx context.Context, name string, r io.Reader, limit int64) (int64, error) {
	if !blob.ValidHash(name) {
		return 0, blob.ErrBadHash
	}

	_, size, err := s.put(ctx, name, r, limit)

	return size, err
}

// put stores r under name, its own hash when name is empty.
func (s *Store) put(ctx context.Context, name string, r io.Reader, limit int64) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	f, hash, size, err := blob.Spool(s.spool, r, limit)
	if err != nil {
		return "", 0, err
	}

	if name != "" {
		hash = name
	}

	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	data, err := io.ReadAll(f)
	if err != nil {
		return "", 0, err
	}

	// DO NOTHING: two uploads of the same content racing here both
	// succeed, whichever row lands.
	_, err = s.Exec(ctx, `INSERT INTO blobs (hash, data, size, created_at) VALUES (?, ?, ?, ?) ON CONFLICT (hash) DO NOTHING`,
		hash, data, size, time.Now().UTC())
	if err != nil {
		return "", 0, err
	}

	return hash, size, nil
}

// Open reads the row, the whole of it, and hands back the part asked
// for.
func (s *Store) Open(ctx context.Context, hash string, rng *blob.Range) (io.ReadCloser, blob.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, blob.Object{}, err
	}

	if !blob.ValidHash(hash) {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	var data []byte
	err := s.QueryRow(ctx, `SELECT data FROM blobs WHERE hash = ?`, hash).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	if err != nil {
		return nil, blob.Object{}, err
	}

	size := int64(len(data))
	obj := blob.Object{Size: size, Offset: 0, Length: size}
	if rng == nil {
		return io.NopCloser(bytesReader(data)), obj, nil
	}

	offset, length, ok := rng.Resolve(size)
	if !ok {
		return nil, obj, blob.ErrRange
	}

	obj.Offset, obj.Length = offset, length

	return io.NopCloser(bytesReader(data[offset : offset+length])), obj, nil
}

// Exists reports whether the row is there.
func (s *Store) Exists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	if !blob.ValidHash(hash) {
		return false, nil
	}

	var n int
	if err := s.QueryRow(ctx, `SELECT COUNT(*) FROM blobs WHERE hash = ?`, hash).Scan(&n); err != nil {
		return false, err
	}

	return n > 0, nil
}

// Delete removes the row. Missing is fine.
func (s *Store) Delete(ctx context.Context, hash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if !blob.ValidHash(hash) {
		return nil
	}

	_, err := s.Exec(ctx, `DELETE FROM blobs WHERE hash = ?`, hash)

	return err
}

// List reads every hash and its time.
func (s *Store) List(ctx context.Context, fn func(hash string, modified time.Time) error) error {
	rows, err := s.Query(ctx, `SELECT hash, created_at FROM blobs`)
	if err != nil {
		return err
	}

	defer rows.Close() //nolint:errcheck // read-only

	for rows.Next() {
		var (
			hash string
			at   time.Time
		)

		if err := rows.Scan(&hash, &at); err != nil {
			return err
		}

		if err := fn(hash, at.UTC()); err != nil {
			return err
		}
	}

	return rows.Err()
}

// Ping checks the table answers.
func (s *Store) Ping(ctx context.Context) error {
	var n int

	return s.QueryRow(ctx, `SELECT COUNT(*) FROM blobs`).Scan(&n)
}
