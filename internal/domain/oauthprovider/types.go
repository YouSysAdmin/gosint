// Gosint, Copyright (c) 2026 YouSysAdmin

package oauthprovider

import (
	"time"

	"github.com/yousysadmin/gosint/internal/core/oidc"
)

// patchInput is the create and update body: every field optional, so
// a PATCH naming one field leaves the others. The client secret is
// never read back, so an absent one keeps the stored one and an empty
// string clears it. A list absent stays, [] clears, the difference
// matters on a PATCH: a body fixing the name must not empty
// allowed_domains with auto_register on.
type patchInput struct {
	Name     *string `json:"name"          validate:"omitnil,max=100"`
	Slug     *string `json:"slug"          validate:"omitnil,max=60"`
	Type     *string `json:"type"          validate:"omitnil,max=20"`
	ClientID *string `json:"client_id"     validate:"omitnil,max=400"`
	Secret   *string `json:"client_secret" validate:"omitnil,max=1000"`

	Issuer      *string `json:"issuer"       validate:"omitnil,max=400"`
	AuthURL     *string `json:"auth_url"     validate:"omitnil,max=400"`
	TokenURL    *string `json:"token_url"    validate:"omitnil,max=400"`
	UserInfoURL *string `json:"userinfo_url" validate:"omitnil,max=400"`

	Scopes []string `json:"scopes" validate:"omitempty,max=20"`

	Enabled      *bool `json:"enabled"`
	Hidden       *bool `json:"hidden"`
	AutoRegister *bool `json:"auto_register"`

	RequireEmailVerified *bool    `json:"require_email_verified"`
	LinkByEmail          *bool    `json:"link_by_email"`
	AllowPrivateTargets  *bool    `json:"allow_private_targets"`
	AllowedDomains       []string `json:"allowed_domains" validate:"omitempty,max=50"`
	AllowedEmails        []string `json:"allowed_emails"  validate:"omitempty,max=200"`
	GroupsClaim          *string  `json:"groups_claim"    validate:"omitnil,max=100"`
	AllowedGroups        []string `json:"allowed_groups"  validate:"omitempty,max=100"`
}

// ProviderView is one provider as the panel sees it. has_secret, never
// the secret.
type ProviderView struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Slug                 string   `json:"slug"`
	Type                 string   `json:"type"`
	ClientID             string   `json:"client_id"`
	HasSecret            bool     `json:"has_secret"`
	Issuer               string   `json:"issuer"`
	AuthURL              string   `json:"auth_url"`
	TokenURL             string   `json:"token_url"`
	UserInfoURL          string   `json:"userinfo_url"`
	Scopes               []string `json:"scopes"`
	Enabled              bool     `json:"enabled"`
	Hidden               bool     `json:"hidden"`
	AutoRegister         bool     `json:"auto_register"`
	RequireEmailVerified bool     `json:"require_email_verified"`
	LinkByEmail          bool     `json:"link_by_email"`
	AllowPrivateTargets  bool     `json:"allow_private_targets"`
	AllowedDomains       []string `json:"allowed_domains"`
	AllowedEmails        []string `json:"allowed_emails"`
	GroupsClaim          string   `json:"groups_claim"`
	AllowedGroups        []string `json:"allowed_groups"`

	// Usable tells why an enabled provider is still not on the sign-in
	// page, CallbackURL is what to register at the IdP.
	Usable      bool      `json:"usable"`
	CallbackURL string    `json:"callback_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// listResponse lists the providers.
type listResponse struct {
	Providers []ProviderView `json:"providers"`
}

// providerResponse answers one provider.
type providerResponse struct {
	Provider ProviderView `json:"provider"`
}

// testResponse answers the Test button.
type testResponse struct {
	Test oidc.TestResult `json:"test"`
}
