// Gosint, Copyright (c) 2026 YouSysAdmin

package database

import (
	"context"
	"database/sql"
)

// Base is the scaffolding every domain store embeds: the handle and
// the placeholder rewrite.
//
//	type Store struct{ database.Base }
//
//	func NewStore(db database.Database) *Store {
//	    return &Store{database.NewBase(db)}
//	}
//
// Every query goes through Q, so no store can send `?` to postgres.
type Base struct {
	db      *sql.DB
	dialect Dialect
}

// NewBase builds the scaffolding around a backend.
func NewBase(d Database) Base {
	return Base{db: d.DB(), dialect: d.Dialect()}
}

// DB is the raw handle, reserved for transactions.
func (b *Base) DB() *sql.DB { return b.db }

// Q rewrites the `?` placeholders for the backend. sqlite takes them
// as written, postgres wants $1..$N.
func (b *Base) Q(query string) string {
	if b.dialect == Postgres {
		return Rebind(query)
	}

	return query
}

// Exec runs a statement.
func (b *Base) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return b.db.ExecContext(ctx, b.Q(query), args...)
}

// Query runs a read returning rows.
func (b *Base) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return b.db.QueryContext(ctx, b.Q(query), args...)
}

// QueryRow runs a single-row read.
func (b *Base) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return b.db.QueryRowContext(ctx, b.Q(query), args...)
}

// Tx runs fn inside a transaction, committing when fn returns nil and
// rolling back otherwise. The deferred rollback also covers a panic in
// fn, which the caller recovers: left open, the transaction would hold
// sqlite's single write lock until the process restarts.
func (b *Base) Tx(ctx context.Context, fn func(tx *Tx) error) error {
	sqlTx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer sqlTx.Rollback() //nolint:errcheck // ErrTxDone after a commit

	if err := fn(&Tx{tx: sqlTx, dialect: b.dialect}); err != nil {
		return err
	}

	return sqlTx.Commit()
}

// serializableAttempts is how many times SerializableTx runs fn before
// a serialization failure is handed back.
const serializableAttempts = 3

// SerializableTx runs fn inside a serializable transaction, so a
// statement whose condition reads other rows cannot pass alongside a
// concurrent one making that condition false. postgres aborts one of
// two such transactions, and the whole of fn runs again, up to three
// times. sqlite's immediate lock already serializes every writer, so
// there it is the plain Tx. fn must be safe to run again.
func (b *Base) SerializableTx(ctx context.Context, fn func(tx *Tx) error) error {
	if b.dialect != Postgres {
		return b.Tx(ctx, fn)
	}

	var err error
	for range serializableAttempts {
		err = b.serializable(ctx, fn)
		if !SerializationFailure(err) {
			return err
		}
	}

	return err
}

func (b *Base) serializable(ctx context.Context, fn func(tx *Tx) error) error {
	sqlTx, err := b.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}

	defer sqlTx.Rollback() //nolint:errcheck // ErrTxDone after a commit

	if err := fn(&Tx{tx: sqlTx, dialect: b.dialect}); err != nil {
		return err
	}

	return sqlTx.Commit()
}

// Tx is a transaction carrying the same placeholder rewrite as Base.
type Tx struct {
	tx      *sql.Tx
	dialect Dialect
}

func (t *Tx) q(query string) string {
	if t.dialect == Postgres {
		return Rebind(query)
	}

	return query
}

// Exec runs a statement inside the transaction.
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, t.q(query), args...)
}

// Query runs a read inside the transaction.
func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, t.q(query), args...)
}

// QueryRow runs a single-row read inside the transaction.
func (t *Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, t.q(query), args...)
}
