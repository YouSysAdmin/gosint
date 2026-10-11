// Gosint, Copyright (c) 2026 YouSysAdmin

// Package oauthprovider is the identity providers an administrator
// configures and the identities linked to accounts. Storage and the
// admin endpoints, the sign-in itself is in domain/auth.
package oauthprovider

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	opmodel "github.com/yousysadmin/gosint/internal/models/oauthprovider"
)

// slugPurpose is the keyed hash a provider's slug is matched by.
const slugPurpose = "gosint/provider-slug"

// Store persists providers. The slug is a keyed hash with its sealed
// copy, the client secret and every setting naming the identity
// provider sealed, only the switches stay in clear.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the store to db and the vault.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

func providerAAD(column, id string) string { return database.AAD("oauth_providers", column, id) }

const providerSelect = `
SELECT id, name, slug_sealed, type, client_id, client_secret, issuer, auth_url, token_url, userinfo_url, scopes,
       enabled, hidden, auto_register,
       require_email_verified, link_by_email, allow_private_targets, allowed_domains, allowed_emails, groups_claim, allowed_groups, created_at, updated_at
FROM oauth_providers`

// row is a provider's text columns as stored, in providerSelect order.
type row struct {
	name, slugSealed, clientID, issuer, authURL, tokenURL, userInfoURL string
	scopes, domains, emails, groupsClaim, groups                       string
}

// texts pairs each sealed column with its stored value.
func (r *row) texts() []struct {
	col string
	v   *string
} {
	return []struct {
		col string
		v   *string
	}{
		{"name", &r.name}, {"client_id", &r.clientID}, {"issuer", &r.issuer}, {"auth_url", &r.authURL},
		{"token_url", &r.tokenURL}, {"userinfo_url", &r.userInfoURL}, {"scopes", &r.scopes},
		{"allowed_domains", &r.domains}, {"allowed_emails", &r.emails}, {"groups_claim", &r.groupsClaim},
		{"allowed_groups", &r.groups},
	}
}

// Get returns one provider by id.
func (s *Store) Get(ctx context.Context, id string) (*opmodel.Provider, error) {
	return s.one(s.QueryRow(ctx, providerSelect+` WHERE id = ?`, id))
}

// GetBySlug is the sign-in lookup, by the slug's keyed hash.
func (s *Store) GetBySlug(ctx context.Context, slug string) (*opmodel.Provider, error) {
	idx, err := s.codec.Index(slugPurpose, slug)
	if err != nil {
		return nil, err
	}

	return s.one(s.QueryRow(ctx, providerSelect+` WHERE slug = ?`, idx))
}

func (s *Store) one(r *sql.Row) (*opmodel.Provider, error) {
	p, err := s.scan(r)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return p, err
}

// List returns every provider, oldest first.
func (s *Store) List(ctx context.Context) ([]opmodel.Provider, error) {
	rows, err := s.Query(ctx, providerSelect+` ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []opmodel.Provider{}
	for rows.Next() {
		p, err := s.scan(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *p)
	}

	return out, rows.Err()
}

// ListLoginable is what the sign-in page may offer: enabled, not
// hidden, complete enough to work.
func (s *Store) ListLoginable(ctx context.Context) ([]opmodel.Provider, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]opmodel.Provider, 0, len(all))
	for _, p := range all {
		if p.Enabled && !p.Hidden && p.Usable() {
			out = append(out, p)
		}
	}

	return out, nil
}

// Put inserts or replaces the provider by id.
func (s *Store) Put(ctx context.Context, p *opmodel.Provider) error {
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}

	p.UpdatedAt = now
	var secret string
	if p.ClientSecret != "" {
		var err error
		if secret, err = s.codec.SealText(providerAAD("client_secret", p.ID), p.ClientSecret); err != nil {
			return err
		}
	}

	r := row{name: p.Name, clientID: p.ClientID, issuer: p.Issuer, authURL: p.AuthURL, tokenURL: p.TokenURL,
		userInfoURL: p.UserInfoURL, scopes: database.MustJSON(list(p.Scopes)), domains: database.MustJSON(list(p.AllowedDomains)),
		emails: database.MustJSON(list(p.AllowedEmails)), groupsClaim: p.GroupsClaim, groups: database.MustJSON(list(p.AllowedGroups))}

	var errs []error
	for _, t := range r.texts() {
		v, err := s.codec.SealText(providerAAD(t.col, p.ID), *t.v)
		errs = append(errs, err)
		*t.v = v
	}

	slug, err1 := s.codec.Index(slugPurpose, p.Slug)
	slugSealed, err2 := s.codec.SealText(providerAAD("slug_sealed", p.ID), p.Slug)
	if err := errors.Join(append(errs, err1, err2)...); err != nil {
		return err
	}

	_, err := s.Exec(ctx, `
		INSERT INTO oauth_providers (id, name, slug, slug_sealed, type, client_id, client_secret, issuer, auth_url,
			token_url, userinfo_url, scopes, enabled, hidden, auto_register, require_email_verified, link_by_email,
			allow_private_targets, allowed_domains, allowed_emails, groups_claim, allowed_groups, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name, slug = excluded.slug, slug_sealed = excluded.slug_sealed, type = excluded.type,
			client_id = excluded.client_id, client_secret = excluded.client_secret,
			issuer = excluded.issuer, auth_url = excluded.auth_url, token_url = excluded.token_url,
			userinfo_url = excluded.userinfo_url, scopes = excluded.scopes, enabled = excluded.enabled,
			hidden = excluded.hidden, auto_register = excluded.auto_register,
			require_email_verified = excluded.require_email_verified, link_by_email = excluded.link_by_email,
			allow_private_targets = excluded.allow_private_targets,
			allowed_domains = excluded.allowed_domains,
			allowed_emails = excluded.allowed_emails, groups_claim = excluded.groups_claim,
			allowed_groups = excluded.allowed_groups, updated_at = excluded.updated_at`,
		p.ID, r.name, slug, slugSealed, p.Type, r.clientID, secret,
		r.issuer, r.authURL, r.tokenURL, r.userInfoURL, r.scopes, p.Enabled, p.Hidden,
		p.AutoRegister, p.RequireEmailVerified, p.LinkByEmail, p.AllowPrivateTargets, r.domains, r.emails, r.groupsClaim, r.groups,
		p.CreatedAt, p.UpdatedAt)

	return err
}

// Delete removes one provider, its identities cascade.
func (s *Store) Delete(ctx context.Context, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM oauth_providers WHERE id = ?`, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// SlugTaken says another provider holds slug.
func (s *Store) SlugTaken(ctx context.Context, slug, exceptID string) (bool, error) {
	idx, err := s.codec.Index(slugPurpose, slug)
	if err != nil {
		return false, err
	}

	var n int
	err = s.QueryRow(ctx, `SELECT COUNT(*) FROM oauth_providers WHERE slug = ? AND id <> ?`, idx, exceptID).Scan(&n)

	return n > 0, err
}

func (s *Store) scan(sc interface{ Scan(...any) error }) (*opmodel.Provider, error) {
	var (
		p      opmodel.Provider
		r      row
		stored string
	)

	if err := sc.Scan(&p.ID, &r.name, &r.slugSealed, &p.Type, &r.clientID, &stored, &r.issuer, &r.authURL, &r.tokenURL,
		&r.userInfoURL, &r.scopes,
		&p.Enabled, &p.Hidden, &p.AutoRegister, &p.RequireEmailVerified, &p.LinkByEmail, &p.AllowPrivateTargets, &r.domains, &r.emails,
		&r.groupsClaim, &r.groups, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}

	var errs []error
	for _, t := range r.texts() {
		v, err := s.codec.OpenText(providerAAD(t.col, p.ID), *t.v)
		errs = append(errs, err)
		*t.v = v
	}

	slug, err := s.codec.OpenText(providerAAD("slug_sealed", p.ID), r.slugSealed)
	errs = append(errs, err)
	p.Slug = slug

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	p.Name, p.ClientID, p.Issuer, p.AuthURL, p.TokenURL, p.UserInfoURL, p.GroupsClaim =
		r.name, r.clientID, r.issuer, r.authURL, r.tokenURL, r.userInfoURL, r.groupsClaim

	secret, err := s.codec.OpenText(providerAAD("client_secret", p.ID), stored)
	if err != nil {
		return nil, err
	}

	p.ClientSecret = secret

	for raw, dst := range map[string]*[]string{r.scopes: &p.Scopes, r.domains: &p.AllowedDomains, r.emails: &p.AllowedEmails, r.groups: &p.AllowedGroups} {
		if err := database.UnmarshalJSON(raw, dst); err != nil {
			return nil, err
		}

		if *dst == nil {
			*dst = []string{}
		}
	}

	return &p, nil
}

// list is never nil, so the column holds [] and not null.
func list(v []string) []string {
	if v == nil {
		return []string{}
	}

	return v
}

// IdentityStore persists the identity links. The subject is a keyed
// hash with its sealed copy, the address and the name sealed.
type IdentityStore struct {
	database.Base
	codec database.Codec
}

// NewIdentityStore binds the identity store to db.
func NewIdentityStore(db database.Database, codec database.Codec) *IdentityStore {
	return &IdentityStore{Base: database.NewBase(db), codec: codec}
}

const subjectPurpose = "gosint/identity-subject"

// identityAAD binds a sealed column to the (provider, subject) pair, not
// the row: a sign-in upserts on the pair and keeps the row there.
func identityAAD(column, providerID, subjectIdx string) string {
	return database.AAD("oauth_identities", column, providerID+":"+subjectIdx)
}

const identitySelect = `
SELECT id, user_id, provider_id, subject, subject_sealed, email, name, created_at, last_login_at
FROM oauth_identities`

// GetBySubject is the sign-in lookup, keyed on the pair.
func (s *IdentityStore) GetBySubject(ctx context.Context, providerID, subject string) (*opmodel.Identity, error) {
	idx, err := s.codec.Index(subjectPurpose, subject)
	if err != nil {
		return nil, err
	}

	id, err := s.scanIdentity(s.QueryRow(ctx, identitySelect+` WHERE provider_id = ? AND subject = ?`, providerID, idx))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return id, err
}

// CountForUser is how many identities the account holds, so an OIDC
// account with none left (its provider was deleted) can be given a
// reset link and become local.
func (s *IdentityStore) CountForUser(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.QueryRow(ctx, `SELECT COUNT(*) FROM oauth_identities WHERE user_id = ?`, userID).Scan(&n)

	return n, err
}

// ListForUser is the account's identities, oldest first.
func (s *IdentityStore) ListForUser(ctx context.Context, userID string) ([]opmodel.Identity, error) {
	return s.list(ctx, identitySelect+` WHERE user_id = ? ORDER BY created_at, id`, userID)
}

func (s *IdentityStore) list(ctx context.Context, query string, args ...any) ([]opmodel.Identity, error) {
	rows, err := s.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []opmodel.Identity{}
	for rows.Next() {
		id, err := s.scanIdentity(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *id)
	}

	return out, rows.Err()
}

// DeleteForUser removes one of the account's identities. Reports
// whether it went, false for an id that is not the account's.
func (s *IdentityStore) DeleteForUser(ctx context.Context, id, userID string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM oauth_identities WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// sealed is an identity's subject index and its sealed subject, address
// and name.
func (s *IdentityStore) sealed(id *opmodel.Identity) ([4]string, error) {
	var (
		c    [4]string
		errs [4]error
	)

	c[0], errs[0] = s.codec.Index(subjectPurpose, id.Subject)
	c[1], errs[1] = s.codec.SealText(identityAAD("subject_sealed", id.ProviderID, c[0]), id.Subject)
	c[2], errs[2] = s.codec.SealText(identityAAD("email", id.ProviderID, c[0]), id.Email)
	c[3], errs[3] = s.codec.SealText(identityAAD("name", id.ProviderID, c[0]), id.Name)

	return c, errors.Join(errs[:]...)
}

// Link records the identity, or refreshes it when the pair signed in
// before. One statement, so two concurrent sign-ins cannot both insert.
func (s *IdentityStore) Link(ctx context.Context, id *opmodel.Identity) error {
	now := time.Now().UTC()
	if id.ID == "" {
		id.ID = ids.New()
	}

	if id.CreatedAt.IsZero() {
		id.CreatedAt = now
	}

	id.LastLoginAt = now
	c, err := s.sealed(id)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO oauth_identities (id, user_id, provider_id, subject, subject_sealed, email, name, created_at, last_login_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider_id, subject) DO UPDATE SET
			subject_sealed = excluded.subject_sealed, email = excluded.email, name = excluded.name,
			last_login_at = excluded.last_login_at`,
		id.ID, id.UserID, id.ProviderID, c[0], c[1], c[2], c[3], id.CreatedAt, id.LastLoginAt)

	return err
}

func (s *IdentityStore) scanIdentity(r interface{ Scan(...any) error }) (*opmodel.Identity, error) {
	var (
		id              opmodel.Identity
		subjectIdx, sub string
	)

	if err := r.Scan(&id.ID, &id.UserID, &id.ProviderID, &subjectIdx, &sub, &id.Email, &id.Name, &id.CreatedAt, &id.LastLoginAt); err != nil {
		return nil, err
	}

	var errs [3]error
	id.Subject, errs[0] = s.codec.OpenText(identityAAD("subject_sealed", id.ProviderID, subjectIdx), sub)
	id.Email, errs[1] = s.codec.OpenText(identityAAD("email", id.ProviderID, subjectIdx), id.Email)
	id.Name, errs[2] = s.codec.OpenText(identityAAD("name", id.ProviderID, subjectIdx), id.Name)

	return &id, errors.Join(errs[:]...)
}
