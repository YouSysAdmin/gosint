// Gosint, Copyright (c) 2026 YouSysAdmin

package database

import (
	"database/sql"
	"time"
)

// Pool bounds a connection pool. A zero field keeps the driver default.
type Pool struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
	MaxIdleTime time.Duration
}

// Apply configures db, leaving driver defaults where a field is 0.
func (p Pool) Apply(db *sql.DB) {
	if p.MaxOpen > 0 {
		db.SetMaxOpenConns(p.MaxOpen)
	}

	if p.MaxIdle > 0 {
		db.SetMaxIdleConns(p.MaxIdle)
	}

	if p.MaxLifetime > 0 {
		db.SetConnMaxLifetime(p.MaxLifetime)
	}

	if p.MaxIdleTime > 0 {
		db.SetConnMaxIdleTime(p.MaxIdleTime)
	}
}
