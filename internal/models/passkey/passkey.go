// Gosint, Copyright (c) 2026 YouSysAdmin

// Package passkey is one enrolled WebAuthn credential on an account.
package passkey

import "time"

// Passkey is a row of user_passkeys. Credential is the go-webauthn
// credential as opaque JSON, the PUBLIC key plus the sign counter,
// there is no secret here. It is still kept off the wire, a credential
// id is a correlatable identifier the console has no reason to hand out.
type Passkey struct {
	ID     string `json:"id"`
	UserID string `json:"-"`

	// CredentialID is the authenticator's raw credential id, base64url.
	CredentialID string `json:"-"`

	// Name is what the account holder called it.
	Name       string     `json:"name"`
	Credential string     `json:"-"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitzero"`
}
