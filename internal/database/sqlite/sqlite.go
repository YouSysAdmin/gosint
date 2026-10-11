// Gosint, Copyright (c) 2026 YouSysAdmin

// Package sqlite is the local backend: modernc's pure Go sqlite through
// database/sql, so the binary stays free of cgo.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"github.com/yousysadmin/gosint/internal/database"
)

// SQLite implements database.Database over a sqlite file.
type SQLite struct {
	db   *sql.DB
	path string
}

// Open opens (creating if needed) the database file at path.
//
// WAL lets readers run beside the single writer, busy_timeout makes a
// second writer wait instead of failing, and _txlock=immediate takes
// the write lock at BEGIN so a transaction cannot fail on upgrade from
// a read lock halfway through.
func Open(ctx context.Context, path string, pool database.Pool) (*SQLite, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	q.Set("_time_format", "sqlite")

	// The path is escaped: a "?", "#" or "%" in a directory name would
	// otherwise end it early or be read as an escape.
	file := (&url.URL{Path: path}).EscapedPath()
	db, err := sql.Open("sqlite", "file:"+file+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	pool.Apply(db)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	slog.Info("sqlite open", "path", path)

	return &SQLite{db: db, path: path}, nil
}

// Close releases the handle.
func (s *SQLite) Close() error { return s.db.Close() }

// DB is the handle.
func (s *SQLite) DB() *sql.DB { return s.db }

// Dialect reports sqlite.
func (s *SQLite) Dialect() database.Dialect { return database.SQLite }
