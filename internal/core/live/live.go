// Gosint, Copyright (c) 2026 YouSysAdmin

// Package live holds a value an administrator changes while the server
// runs, read without a lock on every use.
package live

import "sync/atomic"

// Value is one setting, replaced whole.
type Value[T any] struct {
	p atomic.Pointer[T]
}

// New starts the value at v.
func New[T any](v T) *Value[T] {
	l := &Value[T]{}
	l.Set(v)

	return l
}

// Get is the current value.
func (l *Value[T]) Get() T { return *l.p.Load() }

// Set replaces the value.
func (l *Value[T]) Set(v T) { l.p.Store(&v) }
