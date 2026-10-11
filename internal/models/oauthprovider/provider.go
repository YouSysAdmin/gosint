// Gosint, Copyright (c) 2026 YouSysAdmin

// Package oauthprovider is an identity provider configured at runtime
// and the link between an external identity and an account. Ported
// from mailyard.
package oauthprovider

import (
	"strings"
	"time"
)

// Types. oidc discovers its endpoints from the issuer, google is the
// same protocol with an issuer already known.
const (
	TypeOIDC   = "oidc"
	TypeGoogle = "google"
)

// GoogleIssuer is the discovery issuer of the google type.
const GoogleIssuer = "https://accounts.google.com"

// ValidTypes are the assignable types.
var ValidTypes = map[string]struct{}{TypeOIDC: {}, TypeGoogle: {}}

// DefaultScopes is what a provider gets when none are named: openid
// for an ID token at all, email to key the account on.
var DefaultScopes = []string{"openid", "email", "profile"}

// Provider is one configured identity provider. ClientSecret never
// leaves through the API, the store seals it at rest.
type Provider struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Type         string `json:"type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"-"`

	// Issuer drives discovery. Required for oidc, implied for google.
	Issuer string `json:"issuer"`

	// AuthURL, TokenURL and UserInfoURL are the manual fallback for an
	// IdP without a discovery document.
	AuthURL     string   `json:"auth_url"`
	TokenURL    string   `json:"token_url"`
	UserInfoURL string   `json:"userinfo_url"`
	Scopes      []string `json:"scopes"`
	Enabled     bool     `json:"enabled"`

	// Hidden keeps a working provider off the sign-in page, reachable
	// by direct URL.
	Hidden bool `json:"hidden"`

	// AutoRegister creates an account on first sign-in. Off means the
	// account must exist or be invited.
	AutoRegister bool `json:"auto_register"`

	// Admission, against the verified claims. All empty means the IdP
	// is the only gate.
	RequireEmailVerified bool     `json:"require_email_verified"`
	AllowedDomains       []string `json:"allowed_domains"`
	AllowedEmails        []string `json:"allowed_emails"`
	GroupsClaim          string   `json:"groups_claim"`
	AllowedGroups        []string `json:"allowed_groups"`

	// LinkByEmail lets a verified email sign the person into the
	// existing account holding it. Never an administrator's account,
	// which links its providers itself.
	LinkByEmail bool `json:"link_by_email"`

	// AllowPrivateTargets lets the calls to the IdP reach a private
	// address, a provider inside the network. Off, they are refused.
	AllowPrivateTargets bool `json:"allow_private_targets"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HasSecret says a client secret is stored.
func (p *Provider) HasSecret() bool { return p.ClientSecret != "" }

// EffectiveIssuer is the issuer to discover against.
func (p *Provider) EffectiveIssuer() string {
	if p.Type == TypeGoogle {
		return GoogleIssuer
	}

	return p.Issuer
}

// EffectiveScopes is the configured scopes, or the default set.
func (p *Provider) EffectiveScopes() []string {
	if len(p.Scopes) == 0 {
		return DefaultScopes
	}

	return p.Scopes
}

// Usable says the provider is complete enough to attempt a sign-in.
func (p *Provider) Usable() bool {
	if p.ClientID == "" || p.ClientSecret == "" {
		return false
	}

	if p.EffectiveIssuer() != "" {
		return true
	}

	return p.AuthURL != "" && p.TokenURL != ""
}

// NormalizeSlug reduces a name to a URL-safe slug, which sits in
// /api/auth/oauth/<slug>/start.
func NormalizeSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ' || r == '.':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.TrimRight(b.String(), "-")
}

// Identity links an external subject to an account.
type Identity struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	ProviderID  string    `json:"provider_id"`
	Subject     string    `json:"subject"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	LastLoginAt time.Time `json:"last_login_at"`
}
