// Gosint, Copyright (c) 2026 YouSysAdmin

// Package user is the account record.
package user

import (
	"strings"
	"time"
)

// How an account signs in. A local account holds a password here, an
// OIDC account is the identity provider's and has none.
const (
	AccountLocal = 1
	AccountOIDC  = 2
)

// User is one account.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	FirstName    string    `json:"first_name"`
	LastName     string    `json:"last_name"`
	AvatarURL    string    `json:"avatar_url"`
	Active       bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Admin administers accounts and the install. The first account
	// is one.
	Admin bool `json:"admin"`

	// AccountType is AccountLocal or AccountOIDC.
	AccountType int `json:"account_type"`

	// TOTPSecret is the authenticator secret, sealed and encoded, ""
	// when none was set up. TOTPEnabled says a code proved the
	// authenticator holds it. TOTPLastStep is the last time step a code
	// was accepted for, so a code works once.
	TOTPSecret   string `json:"-"`
	TOTPEnabled  bool   `json:"totp_enabled"`
	TOTPLastStep int64  `json:"-"`

	// Wrong second factor codes and wrong passwords, counted toward a
	// lock that ends at the time named.
	TOTPFailures     int        `json:"-"`
	TOTPLockedUntil  *time.Time `json:"-"`
	LoginFailures    int        `json:"-"`
	LoginLockedUntil *time.Time `json:"-"`
}

// Filter narrows the admin panel's list.
type Filter struct {
	// Query matches anywhere in the email, lowercased.
	Query string

	// Admin and Disabled, when set, keep only accounts with that flag.
	Admin    *bool
	Disabled *bool
	Limit    int
}

// Local says the account holds its own password, so a password, a
// second factor and passkeys are its own to manage.
func (u *User) Local() bool { return u.AccountType != AccountOIDC }

// DisplayName is the name to show for the account, the email when it
// has none.
func (u *User) DisplayName() string {
	if name := strings.TrimSpace(u.FirstName + " " + u.LastName); name != "" {
		return name
	}

	return u.Email
}
