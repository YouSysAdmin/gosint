// Gosint, Copyright (c) 2026 YouSysAdmin

// Package ids mints the primary keys, so the uuid version is decided in
// one place.
package ids

import "uuid"

// New returns a UUIDv7 as a string. v7 keeps inserts at the right-hand
// edge of the index and sorts in creation order.
func New() string {
	return uuid.NewV7().String()
}

// Valid reports whether s is a uuid at all.
func Valid(s string) bool {
	_, err := uuid.Parse(s)

	return err == nil
}
