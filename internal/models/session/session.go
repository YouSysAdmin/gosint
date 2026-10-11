// Gosint, Copyright (c) 2026 YouSysAdmin

// Package session is the sign-in record behind the session cookie.
package session

import "time"

// Session is one sign-in.
type Session struct {
	ID        string
	UserID    string
	TokenHash string
	UserAgent string
	ClientIP  string
	CreatedAt time.Time
	ExpiresAt time.Time
}
