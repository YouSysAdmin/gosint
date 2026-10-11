// Gosint, Copyright (c) 2026 YouSysAdmin

package oauthprovider

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/oidc"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	opmodel "github.com/yousysadmin/gosint/internal/models/oauthprovider"
)

// Handler serves the admin's providers. Every route is behind
// requireSession and requireAdmin.
type Handler struct {
	Runtime *env.Runtime
}

func (h Handler) view(p *opmodel.Provider) ProviderView {
	return ProviderView{
		ID: p.ID, Name: p.Name, Slug: p.Slug, Type: p.Type, ClientID: p.ClientID, HasSecret: p.HasSecret(),
		Issuer: p.Issuer, AuthURL: p.AuthURL, TokenURL: p.TokenURL, UserInfoURL: p.UserInfoURL,
		Scopes: p.EffectiveScopes(), Enabled: p.Enabled, Hidden: p.Hidden, AutoRegister: p.AutoRegister,
		RequireEmailVerified: p.RequireEmailVerified, LinkByEmail: p.LinkByEmail, AllowPrivateTargets: p.AllowPrivateTargets, AllowedDomains: p.AllowedDomains, AllowedEmails: p.AllowedEmails,
		GroupsClaim: p.GroupsClaim, AllowedGroups: p.AllowedGroups, Usable: p.Usable(),
		CallbackURL: h.Runtime.OAuth.RedirectURL(p.Slug), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// List answers every provider.
func (h Handler) List(c fiber.Ctx) error {
	ps, err := h.Runtime.Store.OAuthProvider.List(c)
	if err != nil {
		return response.Internal(c, err)
	}

	out := listResponse{Providers: make([]ProviderView, 0, len(ps))}
	for i := range ps {
		out.Providers = append(out.Providers, h.view(&ps[i]))
	}

	return response.Success(c, out)
}

// Get answers one provider.
func (h Handler) Get(c fiber.Ctx) error {
	p, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	return response.Success(c, providerResponse{Provider: h.view(p)})
}

// Create adds a provider: on, visible, registering accounts, and
// requiring a verified email, the safe default since sign-in links by
// email to an existing account.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchInput](c)
	if !ok {
		return resp
	}

	p := &opmodel.Provider{ID: ids.New(), Enabled: true, AutoRegister: true, RequireEmailVerified: true,
		Scopes: []string{}, AllowedDomains: []string{}, AllowedEmails: []string{}, AllowedGroups: []string{}}
	if resp, ok := h.apply(c, p, in, true); !ok {
		return resp
	}

	if err := h.Runtime.Store.OAuthProvider.Put(c, p); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, providerResponse{Provider: h.view(p)})
}

// Update patches a provider and drops its cached discovery.
func (h Handler) Update(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[patchInput](c)
	if !ok {
		return resp
	}

	p, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	oldSlug := p.Slug
	if resp, ok := h.apply(c, p, in, false); !ok {
		return resp
	}

	if err := h.Runtime.Store.OAuthProvider.Put(c, p); err != nil {
		return response.Internal(c, err)
	}

	h.Runtime.OAuth.Forget(oldSlug)
	h.Runtime.OAuth.Forget(p.Slug)

	return response.Success(c, providerResponse{Provider: h.view(p)})
}

// Delete removes a provider. Its identities cascade, and an account
// that signed in only through it waits for an admin's reset link.
func (h Handler) Delete(c fiber.Ctx) error {
	p, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	if _, err := h.Runtime.Store.OAuthProvider.Delete(c, p.ID); err != nil {
		return response.Internal(c, err)
	}

	h.Runtime.OAuth.Forget(p.Slug)

	return response.NoContent(c)
}

// Test runs discovery against the stored configuration and reports it.
func (h Handler) Test(c fiber.Ctx) error {
	p, resp, ok := h.load(c)
	if !ok {
		return resp
	}

	return response.Success(c, testResponse{Test: h.Runtime.OAuth.Test(c, p)})
}

func (h Handler) load(c fiber.Ctx) (*opmodel.Provider, error, bool) {
	p, err := h.Runtime.Store.OAuthProvider.Get(c, c.Params("id"))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if p == nil {
		return nil, response.NotFound(c, "provider not found"), false
	}

	return p, nil, true
}

// apply folds the input onto the record. The slug is derived from the
// name on create only, it is part of the redirect URI registered at
// the IdP.
func (h Handler) apply(c fiber.Ctx, p *opmodel.Provider, in patchInput, creating bool) (error, bool) {
	if in.Name != nil {
		p.Name = strings.TrimSpace(*in.Name)
	}

	if p.Name == "" {
		return response.BadRequest(c, "name must not be empty"), false
	}

	if in.Type != nil || creating {
		p.Type = strings.ToLower(trimmed(in.Type))
		if p.Type == "" {
			p.Type = opmodel.TypeOIDC
		}
	}

	if _, ok := opmodel.ValidTypes[p.Type]; !ok {
		return response.BadRequest(c, "unknown provider type "+p.Type), false
	}

	if in.Slug != nil || creating {
		slug := opmodel.NormalizeSlug(trimmed(in.Slug))
		if slug == "" && creating {
			slug = opmodel.NormalizeSlug(p.Name)
		}

		if slug == "" {
			return response.BadRequest(c, "the slug is empty after normalization, use letters or digits"), false
		}

		taken, err := h.Runtime.Store.OAuthProvider.SlugTaken(c, slug, p.ID)
		if err != nil {
			return response.Internal(c, err), false
		}

		if taken {
			return response.BadRequest(c, "another provider already uses the slug "+slug), false
		}

		p.Slug = slug
	}

	setText(&p.ClientID, in.ClientID)
	if in.Secret != nil {
		p.ClientSecret = strings.TrimSpace(*in.Secret)
	}

	// The issuer is stored as typed, slash included: it is an identifier
	// the discovery document must match string for string.
	setText(&p.Issuer, in.Issuer)
	setText(&p.AuthURL, in.AuthURL)
	setText(&p.TokenURL, in.TokenURL)
	setText(&p.UserInfoURL, in.UserInfoURL)
	setText(&p.GroupsClaim, in.GroupsClaim)
	for _, u := range []string{p.Issuer, p.AuthURL, p.TokenURL, p.UserInfoURL} {
		if u != "" && !isHTTPURL(u) {
			return response.BadRequest(c, "the URLs must be http(s)"), false
		}
	}

	if in.Enabled != nil {
		p.Enabled = *in.Enabled
	}

	if in.Hidden != nil {
		p.Hidden = *in.Hidden
	}

	if in.AutoRegister != nil {
		p.AutoRegister = *in.AutoRegister
	}

	if in.RequireEmailVerified != nil {
		p.RequireEmailVerified = *in.RequireEmailVerified
	}

	if in.LinkByEmail != nil {
		p.LinkByEmail = *in.LinkByEmail
	}

	if in.AllowPrivateTargets != nil {
		p.AllowPrivateTargets = *in.AllowPrivateTargets
	}

	if in.Scopes != nil {
		p.Scopes = cleanList(in.Scopes, false)
	}

	if in.AllowedDomains != nil {
		p.AllowedDomains = cleanList(in.AllowedDomains, true)
	}

	if in.AllowedEmails != nil {
		p.AllowedEmails = cleanList(in.AllowedEmails, true)
	}

	if in.AllowedGroups != nil {
		p.AllowedGroups = cleanList(in.AllowedGroups, true)
	}

	if p.Type == opmodel.TypeOIDC && p.Issuer == "" && (p.AuthURL == "" || p.TokenURL == "") {
		return response.BadRequest(c, "set an issuer for discovery, or both auth_url and token_url"), false
	}

	if msg := checkIssuerHost(p.Issuer, h.Runtime.Config.Server.PublicURL); msg != "" {
		return response.BadRequest(c, msg), false
	}

	return nil, true
}

func trimmed(v *string) string {
	if v == nil {
		return ""
	}

	return strings.TrimSpace(*v)
}

func setText(dst, v *string) {
	if v != nil {
		*dst = strings.TrimSpace(*v)
	}
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)

	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// checkIssuerHost catches the issuer pointing at this install instead
// of the IdP, which would discover against ourselves and fail at the
// first sign-in with an opaque error.
func checkIssuerHost(issuer, publicURL string) string {
	if issuer == "" || publicURL == "" {
		return ""
	}

	iu, err := url.Parse(issuer)
	if err != nil || iu.Host == "" {
		return ""
	}

	pu, err := url.Parse(publicURL)
	if err != nil || pu.Host == "" {
		return ""
	}

	if strings.EqualFold(iu.Host, pu.Host) {
		return "the issuer points at this install (" + iu.Host + "), it must be the identity provider's own URL"
	}

	return ""
}

// cleanList trims, lowercases when asked, and drops empties.
func cleanList(in []string, lower bool) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if lower {
			s = strings.ToLower(s)
		}

		if s != "" {
			out = append(out, s)
		}
	}

	return out
}

// RedirectURL is exported for the sign-in page's provider list.
func RedirectURL(rt *env.Runtime, slug string) string {
	return strings.TrimRight(rt.Config.Server.PublicURL, "/") + oidc.CallbackPath(slug)
}
