// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// HashInvitationToken is the stored form of an invitation token.
func HashInvitationToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

// InvitationTTL is how long an invitation link stays usable.
const InvitationTTL = 7 * 24 * time.Hour

// Invitation offers an address a place on an investigation. The token
// travels in the link once, only its hash is stored.
type Invitation struct {
	ID              string     `json:"id"`
	InvestigationID string     `json:"investigation_id"`
	Email           string     `json:"email"`
	Role            Role       `json:"role"`
	TokenHash       string     `json:"-"`
	InvitedBy       string     `json:"invited_by"`
	CreatedAt       time.Time  `json:"created_at"`
	ExpiresAt       time.Time  `json:"expires_at"`
	AcceptedAt      *time.Time `json:"accepted_at,omitzero"`
}

// Pending says the invitation can still be accepted at now.
func (i *Invitation) Pending(now time.Time) bool {
	return i.AcceptedAt == nil && now.Before(i.ExpiresAt)
}
