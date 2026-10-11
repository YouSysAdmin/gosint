// Gosint, Copyright (c) 2026 YouSysAdmin

package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// sqliteCoder matches the modernc sqlite Error by method, so this
// package does not pull in the sqlite driver.
type sqliteCoder interface {
	error
	Code() int
}

const (
	pgUniqueViolation = "23505"
	pgSerialization   = "40001"
	pgInvalidTextRepr = "22P02"

	sqliteConstraintUnique     = 2067
	sqliteConstraintPrimaryKey = 1555
)

// UniqueViolation reports whether err is a unique or primary key
// violation on either backend.
func UniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	if e, ok := errors.AsType[*pgconn.PgError](err); ok {
		return e.Code == pgUniqueViolation
	}

	if e, ok := errors.AsType[sqliteCoder](err); ok {
		c := e.Code()

		return c == sqliteConstraintUnique || c == sqliteConstraintPrimaryKey
	}

	return false
}

// MalformedID reports whether err is postgres refusing a value that is
// not a uuid against a uuid column. sqlite stores ids as TEXT, where a
// malformed id simply matches nothing.
//
// Matched on the routine and not only the code: 22P02 also covers a
// malformed integer or json, which are bugs that must stay loud.
func MalformedID(err error) bool {
	if err == nil {
		return false
	}

	e, ok := errors.AsType[*pgconn.PgError](err)

	return ok && e.Code == pgInvalidTextRepr && e.Routine == "string_to_uuid"
}

// SerializationFailure reports whether postgres aborted a serializable
// transaction because a concurrent one conflicted with it. Running the
// transaction again is the answer.
func SerializationFailure(err error) bool {
	e, ok := errors.AsType[*pgconn.PgError](err)

	return ok && e.Code == pgSerialization
}
