// Gosint, Copyright (c) 2026 YouSysAdmin

// Package passwordreset models the single-use token behind the
// forgot-password flow. Only the hash of the token is stored, the
// plaintext exists in the mail and nowhere else, so a database reader
// cannot mint a session out of it.
package passwordreset

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// TTL is how long a reset link stays usable. Short on purpose, the
// token is a whole account if it leaks from a mailbox.
const TTL = 30 * time.Minute

// Token is one outstanding reset request.
type Token struct {
	ID     string
	UserID string

	// TokenHash is hex(sha256(token)).
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time

	// RequestIP records who asked, judged by the per address budget.
	RequestIP string
}

// Usable reports whether the token can still be redeemed at now.
func (t *Token) Usable(now time.Time) bool {
	return t.UsedAt == nil && now.Before(t.ExpiresAt)
}

// Generate mints a token: the plaintext, mailed once, and its hash.
func Generate() (plaintext, hash string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}

	plaintext = hex.EncodeToString(raw)

	return plaintext, Hash(plaintext), nil
}

// Hash is the stored form of a token.
func Hash(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}
