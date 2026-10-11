// Gosint, Copyright (c) 2026 YouSysAdmin

package oidc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// StatePayload is what the state cookie carries between Authorize and
// Exchange: state guards the callback against CSRF, nonce binds the ID
// token to this request, the verifier is the PKCE secret. Invite is
// the invitation token the sign-in started from, "" for none. Link is
// the account a signed-in person is linking the provider to, "" for a
// sign-in. Both are signed and expiring with the rest, so the callback
// trusts them, and hold no dot since the serialization is dot
// separated (the writers check).
type StatePayload struct {
	State        string
	Nonce        string
	CodeVerifier string
	Invite       string
	Link         string
}

// Authorize mints fresh state, nonce and verifier and returns the IdP
// redirect URL plus the signed cookie value the callback recovers them
// from. link is the account id of a linking round trip, "" otherwise.
func (p *Provider) Authorize(key []byte, invite, link string) (redirectURL, cookieValue string) {
	pay := StatePayload{State: randHex(16), Nonce: randHex(16), CodeVerifier: randURLB64(32), Invite: invite,
		Link: strings.ReplaceAll(link, ".", "")}
	cookieValue = signState(pay, key, time.Now().Add(StateCookieTTL))
	redirectURL = p.oauth2.AuthCodeURL(pay.State,
		oauth2.SetAuthURLParam("code_challenge", pkceChallenge(pay.CodeVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		gooidc.Nonce(pay.Nonce),
	)

	return redirectURL, cookieValue
}

// Claims is what the callback decides on.
type Claims struct {
	Subject       string         `json:"sub"`
	Email         string         `json:"email"`
	EmailVerified bool           `json:"email_verified"`
	Name          string         `json:"name"`
	Raw           map[string]any `json:"-"`
}

// Exchange verifies the callback against the state cookie, exchanges
// the code, validates the ID token (signature, audience, nonce,
// expiry) and returns the claims and the verified state, which names
// the invitation and the account being linked.
func (p *Provider) Exchange(ctx context.Context, key []byte, returnedState, code, cookieValue string) (*Claims, StatePayload, error) {
	pay, err := verifyState(cookieValue, key)
	if err != nil {
		return nil, StatePayload{}, fmt.Errorf("state cookie: %w", err)
	}

	if pay.State == "" || pay.State != returnedState {
		return nil, StatePayload{}, errors.New("state mismatch")
	}

	ctx = withClient(ctx, p.client)
	tok, err := p.oauth2.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pay.CodeVerifier))
	if err != nil {
		return nil, StatePayload{}, fmt.Errorf("token exchange: %w", err)
	}

	// No discovery document means no keys, so identity is read from
	// the UserInfo endpoint over the channel the token came from.
	if p.verifier == nil {
		c, uerr := p.userInfoClaims(ctx, tok)

		return c, pay, uerr
	}

	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, StatePayload{}, errors.New("id_token missing from the token response")
	}

	idTok, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, StatePayload{}, fmt.Errorf("id_token verify: %w", err)
	}

	if idTok.Nonce != pay.Nonce {
		return nil, StatePayload{}, errors.New("nonce mismatch")
	}

	var c Claims
	if err := idTok.Claims(&c); err != nil {
		return nil, StatePayload{}, fmt.Errorf("decode id_token claims: %w", err)
	}

	if err := idTok.Claims(&c.Raw); err != nil {
		return nil, StatePayload{}, fmt.Errorf("decode raw claims: %w", err)
	}

	if c.Subject == "" {
		return nil, StatePayload{}, errors.New("id_token has no sub claim")
	}

	return &c, pay, nil
}

// userInfoClaims reads identity from the UserInfo endpoint.
func (p *Provider) userInfoClaims(ctx context.Context, tok *oauth2.Token) (*Claims, error) {
	if p.userInfoURL == "" {
		return nil, errors.New("no id_token verifier and no userinfo_url configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userInfoURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: HTTPTimeout}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))

		return nil, fmt.Errorf("userinfo returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw map[string]any
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &raw); err != nil {
		return nil, fmt.Errorf("decode userinfo: %w", err)
	}

	c := &Claims{Raw: raw}
	c.Subject, _ = raw["sub"].(string)
	c.Email, _ = raw["email"].(string)
	c.Name, _ = raw["name"].(string)
	switch v := raw["email_verified"].(type) {
	case bool:
		c.EmailVerified = v
	case string:
		c.EmailVerified = v == "true"
	}

	if c.Name == "" {
		given, _ := raw["given_name"].(string)
		family, _ := raw["family_name"].(string)
		c.Name = strings.TrimSpace(given + " " + family)
	}

	if c.Name == "" {
		c.Name, _ = raw["preferred_username"].(string)
	}

	if c.Subject == "" {
		return nil, errors.New("userinfo response has no sub claim")
	}

	return c, nil
}

// signState is "state.nonce.verifier.invite.link.exp.sig", HMAC-SHA256
// over the first six fields.
func signState(p StatePayload, key []byte, exp time.Time) string {
	body := strings.Join([]string{p.State, p.Nonce, p.CodeVerifier, p.Invite, p.Link, strconv.FormatInt(exp.Unix(), 10)}, ".")
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))

	return body + "." + hex.EncodeToString(mac.Sum(nil))
}

func verifyState(cookie string, key []byte) (StatePayload, error) {
	parts := strings.Split(cookie, ".")
	if len(parts) != 7 {
		return StatePayload{}, errors.New("malformed cookie")
	}

	body := strings.Join(parts[:6], ".")
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(body))
	got, err := hex.DecodeString(parts[6])
	if err != nil || !hmac.Equal(mac.Sum(nil), got) {
		return StatePayload{}, errors.New("bad signature")
	}

	exp, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil {
		return StatePayload{}, errors.New("bad expiry")
	}

	if time.Now().Unix() > exp {
		return StatePayload{}, errors.New("expired")
	}

	return StatePayload{State: parts[0], Nonce: parts[1], CodeVerifier: parts[2], Invite: parts[3], Link: parts[4]}, nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

func randURLB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return base64.RawURLEncoding.EncodeToString(b)
}
