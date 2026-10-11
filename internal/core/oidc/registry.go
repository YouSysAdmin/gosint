// Gosint, Copyright (c) 2026 YouSysAdmin

package oidc

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	opmodel "github.com/yousysadmin/gosint/internal/models/oauthprovider"
)

// apiPrefix is where the API is mounted. A copy, this package imports
// nothing from the server.
const apiPrefix = "/api"

// StartPath and CallbackPath are the sign-in routes of one provider.
// The IdP has the callback registered, so the shape is pinned here.
func StartPath(slug string) string { return apiPrefix + "/auth/oauth/" + slug + "/start" }

// CallbackPath is where a provider redirects back to.
func CallbackPath(slug string) string { return apiPrefix + "/auth/oauth/" + slug + "/callback" }

// Registry builds and caches a Provider per configured IdP. Discovery
// is an outbound round trip, far too slow per sign-in, and a row is
// editable at runtime, so the cache is keyed on the row's UpdatedAt.
type Registry struct {
	mu        sync.Mutex
	built     map[string]*cached
	publicURL string

	// guarded refuses private addresses, open is for a provider
	// allowed to sit on one.
	guarded, open *http.Client
}

type cached struct {
	provider *Provider
	stamp    time.Time
}

// NewRegistry builds a Registry over NewHTTPClient's two clients.
func NewRegistry(publicURL string) *Registry {
	return &Registry{built: map[string]*cached{}, publicURL: strings.TrimRight(publicURL, "/"),
		guarded: NewHTTPClient(false), open: NewHTTPClient(true)}
}

// clientFor is the client a provider's calls go through.
func (r *Registry) clientFor(p *opmodel.Provider) *http.Client {
	if p.AllowPrivateTargets {
		return r.open
	}

	return r.guarded
}

// For returns a flow-ready Provider for the row, discovering on first
// use. Built on a context without the request's cancellation: the key
// set behind the verifier keeps the context it was built with.
func (r *Registry) For(ctx context.Context, p *opmodel.Provider) (*Provider, error) {
	r.mu.Lock()
	if c, ok := r.built[p.Slug]; ok && c.stamp.Equal(p.UpdatedAt) {
		r.mu.Unlock()

		return c.provider, nil
	}

	r.mu.Unlock()

	built, err := r.build(context.WithoutCancel(ctx), p)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.built[p.Slug] = &cached{provider: built, stamp: p.UpdatedAt}
	r.mu.Unlock()

	return built, nil
}

// Forget drops a cached provider after an edit or a delete.
func (r *Registry) Forget(slug string) {
	r.mu.Lock()
	delete(r.built, slug)
	r.mu.Unlock()
}

// RedirectURL is the callback the operator registers at the IdP.
func (r *Registry) RedirectURL(slug string) string { return r.publicURL + CallbackPath(slug) }

func (r *Registry) build(ctx context.Context, p *opmodel.Provider) (*Provider, error) {
	client := r.clientFor(p)
	ctx = withClient(ctx, client)
	if !p.RequireEmailVerified {
		slog.Warn("oidc: provider does not require email_verified, an unverified address from it links to the local account with that address",
			"provider", p.Slug)
	}

	cfg := Config{
		ClientID: p.ClientID, ClientSecret: p.ClientSecret, RedirectURL: r.RedirectURL(p.Slug),
		Scopes: p.EffectiveScopes(), RequireEmailVerified: p.RequireEmailVerified,
		AllowedDomains: p.AllowedDomains, AllowedEmails: p.AllowedEmails,
		GroupsClaim: p.GroupsClaim, AllowedGroups: p.AllowedGroups,
	}

	if issuer := p.EffectiveIssuer(); issuer != "" {
		cfg.Issuer = issuer
		prov, err := gooidc.NewProvider(ctx, issuer)

		// The spec compares issuers as exact strings and providers
		// disagree with their operators about the trailing slash. The
		// discovery document is the authority on its own issuer, so on
		// that one mismatch the flipped spelling is tried once.
		if err != nil && strings.Contains(err.Error(), "did not match") {
			flipped := strings.TrimSuffix(issuer, "/")
			if flipped == issuer {
				flipped = issuer + "/"
			}

			if prov2, err2 := gooidc.NewProvider(ctx, flipped); err2 == nil {
				prov, err = prov2, nil
				cfg.Issuer = flipped
			}
		}

		if err == nil {
			return &Provider{cfg: cfg, oauth2: oauthConfig(cfg, prov.Endpoint()),
				verifier: prov.Verifier(&gooidc.Config{ClientID: p.ClientID}), client: client}, nil
		}

		if p.AuthURL == "" || p.TokenURL == "" {
			return nil, fmt.Errorf("discovery against %s failed and no auth_url/token_url is configured: %w", issuer, err)
		}
	}

	if p.AuthURL == "" || p.TokenURL == "" {
		return nil, fmt.Errorf("provider %q: %w", p.Slug, ErrNoDiscovery)
	}

	return &Provider{cfg: cfg, oauth2: oauthConfig(cfg, oauth2.Endpoint{AuthURL: p.AuthURL, TokenURL: p.TokenURL}),
		userInfoURL: p.UserInfoURL, client: client}, nil
}

func oauthConfig(cfg Config, ep oauth2.Endpoint) *oauth2.Config {
	return &oauth2.Config{ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
		Endpoint: ep, Scopes: cfg.Scopes}
}

// TestResult is what the admin Test button reports.
type TestResult struct {
	OK          bool     `json:"ok"`
	Error       string   `json:"error,omitzero"`
	Issuer      string   `json:"issuer,omitzero"`
	AuthURL     string   `json:"auth_url,omitzero"`
	TokenURL    string   `json:"token_url,omitzero"`
	Discovered  bool     `json:"discovered"`
	RedirectURL string   `json:"redirect_url"`
	Scopes      []string `json:"scopes"`
	Warnings    []string `json:"warnings"`
}

// Test resolves the provider without caching and reports what came
// back. It cannot verify the client secret, only a real sign-in does.
func (r *Registry) Test(ctx context.Context, p *opmodel.Provider) TestResult {
	res := TestResult{RedirectURL: r.RedirectURL(p.Slug), Scopes: p.EffectiveScopes(), Issuer: p.EffectiveIssuer(), Warnings: []string{}}
	if r.publicURL == "" {
		res.Warnings = append(res.Warnings, "server.public_url is not set, so the callback URL to register cannot be built")
	}

	if p.ClientID == "" {
		res.Warnings = append(res.Warnings, "no client id is set")
	}

	if p.ClientSecret == "" {
		res.Warnings = append(res.Warnings, "no client secret is set")
	}

	built, err := r.build(ctx, p)
	if err != nil {
		res.Error = err.Error()

		return res
	}

	res.OK = true
	res.AuthURL = built.oauth2.Endpoint.AuthURL
	res.TokenURL = built.oauth2.Endpoint.TokenURL
	res.Discovered = built.verifier != nil
	if !res.Discovered {
		res.Warnings = append(res.Warnings, "no discovery document, the id token cannot be verified locally and the userinfo endpoint is used")
		if p.UserInfoURL == "" {
			res.Warnings = append(res.Warnings, "set a userinfo_url, otherwise sign-in cannot read the person's identity")
		}
	}

	if !p.RequireEmailVerified {
		res.Warnings = append(res.Warnings, "require_email_verified is off, an address this IdP has not verified links to the local account carrying it")
	}

	if p.AutoRegister && len(p.AllowedDomains) == 0 && len(p.AllowedEmails) == 0 && len(p.AllowedGroups) == 0 {
		res.Warnings = append(res.Warnings, "auto_register is on with no allowed domains, emails or groups, every account this IdP holds can create one here")
	}

	if !p.Enabled {
		res.Warnings = append(res.Warnings, "the provider is disabled, it is not offered at sign-in")
	}

	if p.Hidden {
		res.Warnings = append(res.Warnings, "the provider is hidden, reachable only by its direct URL")
	}

	return res
}
