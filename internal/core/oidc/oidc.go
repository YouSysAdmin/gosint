// Gosint, Copyright (c) 2026 YouSysAdmin

// Package oidc is the sign-in through an identity provider, ported from
// mailyard: discovery from the issuer, the authorization code redirect
// with PKCE, and ID token verification on the callback. State, nonce
// and the PKCE verifier ride the round trip in a short signed cookie,
// no server-side state.
package oidc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// Config is one resolved provider.
type Config struct {
	Issuer               string
	ClientID             string
	ClientSecret         string
	RedirectURL          string
	Scopes               []string
	RequireEmailVerified bool
	AllowedDomains       []string
	AllowedEmails        []string
	GroupsClaim          string
	AllowedGroups        []string
}

// StateCookie carries state, nonce and verifier across the IdP round
// trip, for StateCookieTTL: long enough for an MFA prompt there.
const (
	StateCookie    = "gosint_oidc_state"
	StateCookieTTL = 10 * time.Minute
)

// Provider bundles the discovery result, an oauth2.Config and an ID
// token verifier. Built and cached by the Registry.
type Provider struct {
	cfg      Config
	oauth2   *oauth2.Config
	verifier *gooidc.IDTokenVerifier

	// userInfoURL is set on the manual endpoint path only, where no
	// discovery document means no keys to verify an ID token against.
	userInfoURL string
	client      *http.Client
}

// Verifies reports whether the provider validates ID tokens locally.
func (p *Provider) Verifies() bool { return p.verifier != nil }

// Config is the resolved config.
func (p *Provider) Config() Config { return p.cfg }

// HTTPTimeout bounds every call to an identity provider.
const HTTPTimeout = 15 * time.Second

// NewHTTPClient is the one client every outbound OIDC call uses. The
// dial refuses private and reserved addresses (`transform.Public`) unless
// allowPrivate says the IdP sits on such a one, and redirects are
// followed, since issuers 301 to their canonical spelling and every hop
// pays the dial check.
func NewHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{Timeout: HTTPTimeout, KeepAlive: 30 * time.Second}
	if !allowPrivate {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}

			ip, err := netip.ParseAddr(host)
			if err != nil || !transform.Public(ip) {
				return fmt.Errorf("%w: %s", transform.ErrPrivateTarget, host)
			}

			return nil
		}
	}

	return &http.Client{
		Timeout: HTTPTimeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// withClient puts the client where go-oidc and oauth2 both look for
// it. Nil leaves the context alone, for a test wanting the default.
func withClient(ctx context.Context, client *http.Client) context.Context {
	if client == nil {
		return ctx
	}

	return gooidc.ClientContext(ctx, client)
}

// IssuerHost is the issuer's host, for "Sign in with <host>".
func (p *Provider) IssuerHost() string {
	u, err := url.Parse(p.cfg.Issuer)
	if err != nil || u.Host == "" {
		return p.cfg.Issuer
	}

	return u.Host
}

// ErrNoDiscovery is a provider with neither an issuer nor endpoints.
var ErrNoDiscovery = errors.New("oidc: neither an issuer nor explicit endpoints")
