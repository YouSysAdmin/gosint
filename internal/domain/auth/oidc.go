// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	coreoidc "github.com/yousysadmin/gosint/internal/core/oidc"
	"github.com/yousysadmin/gosint/internal/core/response"
	opmodel "github.com/yousysadmin/gosint/internal/models/oauthprovider"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// Sign-in through an identity provider, ported from mailyard. A
// provider admits a PERSON, an invitation admits them to an
// investigation and carries their role: satisfying an allow list is
// never a privilege grant, and the first local account stays the only
// administrator by default.

// errNoProvider is an unknown or disabled provider, reported as missing
// so an unauthenticated caller cannot enumerate them.
var errNoProvider = errors.New("provider not found")

// errLinkRequired is an existing account holding the address that the
// provider may not open by email: its owner signs in and links the
// provider from the profile. A second account for the address is
// never made.
var errLinkRequired = errors.New("an account holds the address and the provider may not open it by email")

// resolveProvider loads the :slug provider and builds its flow.
func (h Handler) resolveProvider(c fiber.Ctx) (*opmodel.Provider, *coreoidc.Provider, error) {
	row, err := h.Runtime.Store.OAuthProvider.GetBySlug(c, c.Params("slug"))
	if err != nil {
		return nil, nil, err
	}

	if row == nil || !row.Enabled {
		return nil, nil, errNoProvider
	}

	flow, err := h.Runtime.OAuth.For(c, row)
	if err != nil {
		return row, nil, err
	}

	return row, flow, nil
}

// OAuthStart mints state, nonce and the PKCE verifier, sets the signed
// cookie and sends the browser to the IdP. The slug rides in the
// cookie too, so the callback cannot be pointed at another provider.
func (h Handler) OAuthStart(c fiber.Ctx) error {
	row, flow, err := h.resolveProvider(c)
	if errors.Is(err, errNoProvider) {
		return redirectLogin(c, "sso_unknown_provider")
	}

	if err != nil {
		h.Runtime.Log.Warn("auth: oidc provider failed to build", "provider", c.Params("slug"), "err", err)

		return redirectLogin(c, "sso_provider_error")
	}

	key, err := h.Runtime.StateKey()
	if err != nil {
		return redirectLogin(c, "sso_provider_error")
	}

	redirect, cookieValue := flow.Authorize(key, inviteToken(c.Query("invite")), "")
	c.Cookie(stateCookie(c, h.Runtime, row.Slug+"|"+cookieValue, coreoidc.StateCookieTTL))

	return c.Redirect().Status(fiber.StatusFound).To(redirect)
}

// OAuthLink starts the round trip that links a provider to the signed-in
// account. A POST answering the IdP's URL, which the console follows:
// the request carries the session and passes the cross-site guard, and
// the state cookie names the account, so the callback attaches the
// identity to it and to no one else.
func (h Handler) OAuthLink(c fiber.Ctx) error {
	row, flow, err := h.resolveProvider(c)
	if errors.Is(err, errNoProvider) {
		return response.NotFound(c, "provider not found")
	}

	if err != nil {
		h.Runtime.Log.Warn("auth: oidc provider failed to build", "provider", c.Params("slug"), "err", err)

		return response.Unavailable(c, "the provider cannot be reached, try again later")
	}

	key, err := h.Runtime.StateKey()
	if err != nil {
		return response.Internal(c, err)
	}

	redirect, cookieValue := flow.Authorize(key, "", access.User(c).ID)
	c.Cookie(stateCookie(c, h.Runtime, row.Slug+"|"+cookieValue, coreoidc.StateCookieTTL))

	return response.Success(c, linkStartResponse{RedirectURL: redirect})
}

// OAuthCallback completes the round trip: the state cookie, the code
// exchange, the ID token, the allow lists, the account, then the
// session or the second factor. A refusal shows a generic code, the
// reason stays in the log.
func (h Handler) OAuthCallback(c fiber.Ctx) error {
	row, flow, err := h.resolveProvider(c)
	if errors.Is(err, errNoProvider) {
		return redirectLogin(c, "sso_unknown_provider")
	}

	if err != nil {
		h.Runtime.Log.Warn("auth: oidc provider failed to build", "provider", c.Params("slug"), "err", err)

		return redirectLogin(c, "sso_provider_error")
	}

	defer c.Cookie(stateCookie(c, h.Runtime, "", -time.Hour))
	if e := c.Query("error"); e != "" {
		h.Runtime.Log.Warn("auth: oidc provider returned an error", "provider", row.Slug, "error", e, "description", c.Query("error_description"))

		return redirectLogin(c, "sso_idp_error")
	}

	state, code := c.Query("state"), c.Query("code")
	if state == "" || code == "" {
		return redirectLogin(c, "sso_bad_callback")
	}

	raw := c.Cookies(coreoidc.StateCookie)
	if raw == "" {
		return redirectLogin(c, "sso_state_missing")
	}

	cookieSlug, cookieValue, ok := strings.Cut(raw, "|")
	if !ok || cookieSlug != row.Slug {
		return redirectLogin(c, "sso_state_mismatch")
	}

	key, err := h.Runtime.StateKey()
	if err != nil {
		return redirectLogin(c, "sso_provider_error")
	}

	claims, pay, err := flow.Exchange(c, key, state, code, cookieValue)
	if err != nil {
		h.Runtime.Log.Warn("auth: oidc exchange failed", "provider", row.Slug, "err", err)

		return redirectLogin(c, "sso_token_invalid")
	}

	refuse := redirectLogin
	if pay.Link != "" {
		refuse = redirectProfile
	}

	if err := flow.Admit(claims); err != nil {
		h.Runtime.Log.Warn("auth: oidc sign-in denied", "provider", row.Slug, "sub", claims.Subject, "reason", err)

		return refuse(c, "sso_access_denied")
	}

	if pay.Link != "" {
		return h.finishLink(c, row, claims, pay.Link)
	}

	invite := pay.Invite
	u, err := h.findOrCreateOAuthUser(c, row, claims, invite)
	if errors.Is(err, errLinkRequired) {
		h.Runtime.Log.Warn("auth: oidc account not admitted", "provider", row.Slug, "reason", err)

		return redirectLogin(c, "sso_link_required")
	}

	if err != nil {
		h.Runtime.Log.Warn("auth: oidc account not admitted", "provider", row.Slug, "reason", err)

		return redirectLogin(c, "sso_no_account")
	}

	if !u.Active {
		return redirectLogin(c, "sso_access_denied")
	}

	// An account with a second factor owes it whoever proved the first,
	// the provider's word alone opens no session.
	if u.TOTPEnabled {
		if err := sealCookie(c, h.Runtime, twoFactorCookie, pendingSecondFactor{UserID: u.ID, Invite: invite}, twoFactorTTL); err != nil {
			return response.Internal(c, err)
		}

		return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/login?sso_2fa=1")
	}

	if err := startSession(c, h.Runtime, u); err != nil {
		return response.Internal(c, err)
	}

	if invite != "" {
		return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/invitations/" + invite)
	}

	return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/")
}

// findOrCreateOAuthUser maps the claims to an account: the identity
// link first (it survives an email change at the IdP), then the email
// when the provider verified it, then a new account when the provider
// registers or an invitation admits the address. Never CreateFirst:
// SSO grants no admin.
func (h Handler) findOrCreateOAuthUser(ctx context.Context, prov *opmodel.Provider, claims *coreoidc.Claims, invite string) (*usermodel.User, error) {
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	ident, err := h.Runtime.Store.OAuthIdentity.GetBySubject(ctx, prov.ID, claims.Subject)
	if err != nil {
		return nil, err
	}

	if ident != nil {
		u, err := h.Runtime.Store.User.Get(ctx, ident.UserID)
		if err != nil {
			return nil, err
		}

		if u != nil {
			if u.Active {
				h.linkIdentity(ctx, prov, u, claims)
			}

			return u, nil
		}
	}

	if email != "" {
		existing, err := h.Runtime.Store.User.GetByEmail(ctx, email)
		if err != nil {
			return nil, err
		}

		if existing != nil {
			// Linking by email trusts the IdP about the mailbox, so it
			// takes a provider the administrator allowed to, a verified
			// address, and never an administrator's account: a provider
			// that lets people claim addresses would otherwise hand out
			// the install. Anything else waits for the owner to link.
			switch {
			case !prov.LinkByEmail:
				return nil, fmt.Errorf("%w: the provider does not link by email", errLinkRequired)
			case !prov.RequireEmailVerified || !claims.EmailVerified:
				return nil, fmt.Errorf("%w: the provider did not verify the address", errLinkRequired)
			case existing.Admin:
				return nil, fmt.Errorf("%w: the account is an administrator", errLinkRequired)
			}

			if existing.Active {
				h.linkIdentity(ctx, prov, existing, claims)
			}

			return existing, nil
		}
	}

	if email == "" {
		return nil, errors.New("no email claim and no linked identity")
	}

	if !prov.AutoRegister {
		invited, err := h.invited(ctx, invite, email)
		if err != nil {
			return nil, err
		}

		if !invited {
			return nil, errors.New("no account for the address and the provider does not register accounts")
		}
	}

	now := time.Now().UTC()
	fresh := &usermodel.User{ID: ids.New(), Email: email, AccountType: usermodel.AccountOIDC, Active: true,
		CreatedAt: now, UpdatedAt: now}
	if given, family := strings.Fields(claims.Name), ""; len(given) > 0 {
		fresh.FirstName = given[0]
		if len(given) > 1 {
			family = strings.Join(given[1:], " ")
		}

		fresh.LastName = family
	}

	if err := h.Runtime.Store.User.Create(ctx, fresh); err != nil {
		return nil, err
	}

	h.linkIdentity(ctx, prov, fresh, claims)

	return fresh, nil
}

// linkIdentity records or refreshes the external identity. A failure
// does not fail the sign-in, the next one resolves by email again.
func (h Handler) linkIdentity(ctx context.Context, prov *opmodel.Provider, u *usermodel.User, claims *coreoidc.Claims) {
	err := h.Runtime.Store.OAuthIdentity.Link(ctx, &opmodel.Identity{UserID: u.ID, ProviderID: prov.ID,
		Subject: claims.Subject, Email: claims.Email, Name: claims.Name})
	if err != nil {
		h.Runtime.Log.Warn("auth: oidc identity link failed", "user", u.ID, "provider", prov.Slug, "err", err)
	}
}

// finishLink attaches the identity to the account the round trip was
// started for, which must still be the one signed in: a link cookie
// carried into another person's browser links nothing. An identity
// another account holds stays with it.
func (h Handler) finishLink(c fiber.Ctx, prov *opmodel.Provider, claims *coreoidc.Claims, userID string) error {
	u, _, _, err := Resolve(c, h.Runtime, c.Cookies(CookieName))
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || u.ID != userID {
		h.Runtime.Log.Warn("auth: oidc link refused, the session is not the one that started it", "provider", prov.Slug)

		return redirectProfile(c, "sso_link_session")
	}

	ident, err := h.Runtime.Store.OAuthIdentity.GetBySubject(c, prov.ID, claims.Subject)
	if err != nil {
		return response.Internal(c, err)
	}

	if ident != nil && ident.UserID != u.ID {
		h.Runtime.Log.Warn("auth: oidc link refused, the identity belongs to another account", "provider", prov.Slug, "user", u.ID)

		return redirectProfile(c, "sso_identity_taken")
	}

	if err := h.Runtime.Store.OAuthIdentity.Link(c, &opmodel.Identity{UserID: u.ID, ProviderID: prov.ID,
		Subject: claims.Subject, Email: claims.Email, Name: claims.Name}); err != nil {
		return response.Internal(c, err)
	}

	return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/profile?linked=" + url.QueryEscape(prov.Slug))
}

// inviteToken keeps only what an invitation token can be, 64 hex
// characters, so a caller cannot steer the redirect after sign-in and
// the state serialization meets no dot.
func inviteToken(raw string) string {
	if len(raw) != 64 {
		return ""
	}

	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return ""
		}
	}

	return raw
}

// loginProviders is what the sign-in page offers.
func (h Handler) loginProviders(ctx context.Context) ([]loginProvider, error) {
	rows, err := h.Runtime.Store.OAuthProvider.ListLoginable(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]loginProvider, 0, len(rows))
	for _, p := range rows {
		out = append(out, loginProvider{Slug: p.Slug, Name: p.Name, StartURL: coreoidc.StartPath(p.Slug)})
	}

	return out, nil
}

// redirectLogin sends the browser to the sign-in page with a generic
// code the console turns into a sentence.
func redirectLogin(c fiber.Ctx, code string) error {
	return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/login?err=" + code)
}

// redirectProfile sends a linking round trip back to the profile with
// a code the console turns into a sentence.
func redirectProfile(c fiber.Ctx, code string) error {
	return c.Redirect().Status(fiber.StatusFound).To(env.ConsolePath + "/profile?err=" + code)
}

// stateCookie is the signed round-trip cookie. Lax, not Strict, so the
// IdP's redirect carries it back.
func stateCookie(c fiber.Ctx, rt *env.Runtime, value string, ttl time.Duration) *fiber.Cookie {
	return &fiber.Cookie{Name: coreoidc.StateCookie, Value: value, Path: "/", HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode, Secure: secureCookie(c, rt), Expires: time.Now().Add(ttl)}
}
