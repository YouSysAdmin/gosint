// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// fakeIDP is an OpenID provider on the loopback: a discovery document,
// a key set, and a token endpoint signing an ID token for one person
// with the nonce the authorization request carried.
type fakeIDP struct {
	srv    *httptest.Server
	key    *rsa.PrivateKey
	email  string
	nonce  string
	verify bool
}

func newFakeIDP(t *testing.T) *fakeIDP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	idp := &fakeIDP{key: key, email: "jane@corp.example", verify: true}
	mux := http.NewServeMux()
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{
			"issuer": idp.srv.URL, "authorization_endpoint": idp.srv.URL + "/auth",
			"token_endpoint": idp.srv.URL + "/token", "jwks_uri": idp.srv.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "k1"))
		if err != nil {
			http.Error(w, err.Error(), 500)

			return
		}

		// The client id arrives as Basic auth or in the form, the audience
		// is the configured one either way.
		clientID := r.Form.Get("client_id")
		if u, _, ok := r.BasicAuth(); ok {
			clientID = u
		}

		claims, _ := json.Marshal(map[string]any{
			"iss": idp.srv.URL, "sub": "sub-" + idp.email, "aud": clientID, "exp": time.Now().Add(time.Hour).Unix(),
			"iat": time.Now().Unix(), "nonce": idp.nonce, "email": idp.email, "email_verified": idp.verify, "name": "Jane Roe",
		})
		signed, err := signer.Sign(claims)
		if err != nil {
			http.Error(w, err.Error(), 500)

			return
		}

		idToken, _ := signed.CompactSerialize()
		w.Header().Set("Content-Type", "application/json")
		_ = json.MarshalWrite(w, map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idToken})
	})

	return idp
}

// A provider configured by the admin signs a person in: start redirects
// to the IdP with PKCE, the callback verifies the token and opens a
// session for a new OIDC account, the next sign-in links the same
// account, auto_register off refuses a stranger, a mismatched state is
// refused, and the provider's secret never leaves through the API.
func TestOIDCSignInAgainstAFakeIdP(t *testing.T) {
	idp := newFakeIDP(t)
	srv := newAPI(t, dbtest.SQLite(t), func(cfg *env.Config) {
		cfg.Server.PublicURL = "http://example.test"
		cfg.Auth.OpenRegistration = false
	})
	alice := signUp(t, srv, "alice@example.com")

	var created struct {
		Provider struct {
			ID          string `json:"id"`
			Slug        string `json:"slug"`
			HasSecret   bool   `json:"has_secret"`
			CallbackURL string `json:"callback_url"`
			Usable      bool   `json:"usable"`
		} `json:"provider"`
	}

	code, body, _ := alice.raw("POST", "/api/admin/oauth-providers", map[string]any{
		"name": "Corp IdP", "issuer": idp.srv.URL, "client_id": "cid", "client_secret": "s3cret-value", "auto_register": true,
		"allow_private_targets": true,
	})
	if code != http.StatusCreated || strings.Contains(body, "s3cret-value") {
		t.Fatalf("create = %d %s", code, body)
	}

	_ = json.Unmarshal([]byte(body), &created)
	if created.Provider.Slug != "corp-idp" || !created.Provider.HasSecret || !created.Provider.Usable ||
		created.Provider.CallbackURL != "http://example.test/api/auth/oauth/corp-idp/callback" {
		t.Fatalf("provider = %+v", created.Provider)
	}

	// The fake IdP is on the loopback, which the provider reaches only
	// while it is allowed to.
	var tested struct {
		Test struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"test"`
	}

	providerPath := "/api/admin/oauth-providers/" + created.Provider.ID
	alice.expect(http.StatusOK, "PATCH", providerPath, map[string]any{"allow_private_targets": false})
	alice.expect(http.StatusOK, "POST", providerPath+"/test", nil, &tested)
	if tested.Test.OK || !strings.Contains(tested.Test.Error, "internal address") {
		t.Errorf("a loopback IdP without the switch = %+v", tested.Test)
	}

	alice.expect(http.StatusOK, "PATCH", providerPath, map[string]any{"allow_private_targets": true})

	var test struct {
		Test struct {
			OK         bool `json:"ok"`
			Discovered bool `json:"discovered"`
		} `json:"test"`
	}

	alice.expect(http.StatusOK, "POST", "/api/admin/oauth-providers/"+created.Provider.ID+"/test", nil, &test)
	if !test.Test.OK || !test.Test.Discovered {
		t.Errorf("test = %+v", test.Test)
	}

	var info struct {
		Providers []struct {
			Slug     string `json:"slug"`
			StartURL string `json:"start_url"`
		} `json:"providers"`
	}

	nobody := &api{t: t, srv: srv}
	nobody.expect(http.StatusOK, "GET", "/api/auth/info", nil, &info)
	if len(info.Providers) != 1 || info.Providers[0].StartURL != "/api/auth/oauth/corp-idp/start" {
		t.Fatalf("info providers = %+v", info.Providers)
	}

	// Start: a redirect to the IdP with a challenge, and the state cookie.
	status, _, hdr := rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/start", nil, nil)
	loc, _ := url.Parse(hdr.Get("Location"))
	stateCookie := cookieNamed(hdr, "gosint_oidc_state")
	if status != http.StatusFound || loc == nil || !strings.HasPrefix(loc.String(), idp.srv.URL+"/auth") ||
		loc.Query().Get("code_challenge_method") != "S256" || stateCookie == "" {
		t.Fatalf("start = %d %s cookie %q", status, hdr.Get("Location"), stateCookie)
	}

	idp.nonce = loc.Query().Get("nonce")
	state := loc.Query().Get("state")

	// A callback whose state does not match is refused.
	status, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/callback?state=other&code=c", nil, map[string]string{"gosint_oidc_state": stateCookie})
	if status != http.StatusFound || !strings.Contains(hdr.Get("Location"), "err=sso_") {
		t.Errorf("mismatched state = %d %s", status, hdr.Get("Location"))
	}

	// The real callback opens a session for a new OIDC account.
	status, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/callback?state="+state+"&code=c", nil, map[string]string{"gosint_oidc_state": stateCookie})
	session := cookieNamed(hdr, auth.CookieName)
	if status != http.StatusFound || hdr.Get("Location") != "/app/" || session == "" {
		t.Fatalf("callback = %d %s session %q", status, hdr.Get("Location"), session)
	}

	jane := &api{t: t, srv: srv, cookie: session}
	var me struct {
		User struct {
			Email       string `json:"email"`
			AccountType int    `json:"account_type"`
			Admin       bool   `json:"admin"`
			FirstName   string `json:"first_name"`
		} `json:"user"`
	}

	jane.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
	if me.User.Email != "jane@corp.example" || me.User.AccountType != 2 || me.User.Admin || me.User.FirstName != "Jane" {
		t.Errorf("the provisioned account = %+v", me.User)
	}

	// An OIDC account holds no password: a password change is refused,
	// and the admin's reset link too.
	jane.expect(http.StatusForbidden, "POST", "/api/auth/password", map[string]string{"current_password": "x", "password": "new horse battery"})

	// Second sign-in links the same account, no duplicate.
	_, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/start", nil, nil)
	loc, _ = url.Parse(hdr.Get("Location"))
	idp.nonce = loc.Query().Get("nonce")
	status, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/callback?state="+loc.Query().Get("state")+"&code=c", nil,
		map[string]string{"gosint_oidc_state": cookieNamed(hdr, "gosint_oidc_state")})
	if status != http.StatusFound || cookieNamed(hdr, auth.CookieName) == "" {
		t.Fatalf("second callback = %d %s", status, hdr.Get("Location"))
	}

	var list struct {
		Total int `json:"total"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/users", nil, &list)
	if list.Total != 2 {
		t.Errorf("accounts = %d, want alice and jane", list.Total)
	}

	// auto_register off refuses a stranger, and the stored secret is kept
	// through a patch that does not name it.
	alice.expect(http.StatusOK, "PATCH", "/api/admin/oauth-providers/"+created.Provider.ID, map[string]any{"auto_register": false})
	alice.expect(http.StatusOK, "GET", "/api/admin/oauth-providers/"+created.Provider.ID, nil, &created)
	if !created.Provider.HasSecret {
		t.Error("a patch without the secret cleared it")
	}

	idp.email = "stranger@corp.example"
	_, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/start", nil, nil)
	loc, _ = url.Parse(hdr.Get("Location"))
	idp.nonce = loc.Query().Get("nonce")
	status, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/callback?state="+loc.Query().Get("state")+"&code=c", nil,
		map[string]string{"gosint_oidc_state": cookieNamed(hdr, "gosint_oidc_state")})
	if status != http.StatusFound || !strings.Contains(hdr.Get("Location"), "err=sso_no_account") {
		t.Errorf("a stranger with auto_register off = %d %s", status, hdr.Get("Location"))
	}

	// Deleting the provider takes the identities with it, the list is empty.
	alice.expect(http.StatusNoContent, "DELETE", "/api/admin/oauth-providers/"+created.Provider.ID, nil)
	nobody.expect(http.StatusOK, "GET", "/api/auth/info", nil, &info)
	if len(info.Providers) != 0 {
		t.Errorf("providers after delete = %+v", info.Providers)
	}

	_, _, hdr = rawWith(t, nobody, "GET", "/api/auth/oauth/corp-idp/start", nil, nil)
	if !strings.Contains(hdr.Get("Location"), "err=sso_unknown_provider") {
		t.Errorf("start on a deleted provider = %s", hdr.Get("Location"))
	}
}

// oidcRound runs one round trip against the fake IdP for its current
// email and answers the callback's redirect and the session it opened,
// "" for none. by is nil for a sign-in, else the account linking the
// provider, and finish is the account whose session the callback
// carries, by itself unless a test hands the cookie to someone else.
func oidcRound(t *testing.T, a *api, idp *fakeIDP, by, finish *api) (string, string) {
	t.Helper()

	var to string
	var hdr http.Header
	if by == nil {
		_, _, hdr = rawWith(t, a, "GET", "/api/auth/oauth/corp-idp/start", nil, nil)
		to = hdr.Get("Location")
	} else {
		var status int
		var body string
		status, body, hdr = rawWith(t, a, "POST", "/api/auth/oauth/corp-idp/link", nil, map[string]string{auth.CookieName: by.cookie})
		var start struct {
			RedirectURL string `json:"redirect_url"`
		}

		if status != http.StatusOK || json.Unmarshal([]byte(body), &start) != nil {
			t.Fatalf("link start = %d %s", status, body)
		}

		to = start.RedirectURL
	}

	loc, err := url.Parse(to)
	if err != nil || loc.Query().Get("state") == "" {
		t.Fatalf("start redirect = %q", to)
	}

	idp.nonce = loc.Query().Get("nonce")
	back := map[string]string{"gosint_oidc_state": cookieNamed(hdr, "gosint_oidc_state")}
	if finish != nil {
		back[auth.CookieName] = finish.cookie
	}

	_, _, hdr = rawWith(t, a, "GET", "/api/auth/oauth/corp-idp/callback?state="+loc.Query().Get("state")+"&code=c", nil, back)

	return hdr.Get("Location"), cookieNamed(hdr, auth.CookieName)
}

// An existing account is reached by a provider's verified email only
// when the administrator allowed it on that provider, and never when
// the account is an administrator: those link the provider themselves,
// signed in. A link lands on the account that started it, an identity
// another account holds stays there, and the last way into an account
// cannot be unlinked.
func TestOIDCLinksAccountsOnlyAsAllowed(t *testing.T) {
	idp := newFakeIDP(t)
	srv := newAPI(t, dbtest.SQLite(t), func(cfg *env.Config) {
		cfg.Server.PublicURL = "http://example.test"
		cfg.Auth.OpenRegistration = true
	})
	alice := signUp(t, srv, "alice@corp.example")
	bob := signUp(t, srv, "bob@corp.example")
	carol := signUp(t, srv, "carol@example.com")
	dave := signUp(t, srv, "dave@example.com")
	nobody := &api{t: t, srv: srv}

	var created struct {
		Provider struct {
			ID          string `json:"id"`
			LinkByEmail bool   `json:"link_by_email"`
		} `json:"provider"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/admin/oauth-providers", map[string]any{
		"name": "Corp IdP", "issuer": idp.srv.URL, "client_id": "cid", "client_secret": "s3cret-value", "auto_register": true,
		"allow_private_targets": true,
	}, &created)
	if created.Provider.LinkByEmail {
		t.Fatal("a new provider links by email")
	}

	accounts := func() int {
		var list struct {
			Total int `json:"total"`
		}

		alice.expect(http.StatusOK, "GET", "/api/admin/users", nil, &list)

		return list.Total
	}

	// Off by default: bob's verified address does not open his account,
	// and no second account is made for it.
	idp.email = "bob@corp.example"
	if to, session := oidcRound(t, nobody, idp, nil, nil); !strings.Contains(to, "err=sso_link_required") || session != "" {
		t.Errorf("link by email while off = %s session %q", to, session)
	}

	if n := accounts(); n != 4 {
		t.Errorf("accounts = %d, want 4", n)
	}

	// On, an unverified address still opens nothing: the provider's
	// admission refuses it before any account is looked up.
	alice.expect(http.StatusOK, "PATCH", "/api/admin/oauth-providers/"+created.Provider.ID, map[string]any{"link_by_email": true}, &created)
	if !created.Provider.LinkByEmail {
		t.Fatal("link_by_email did not stick")
	}

	idp.verify = false
	if to, session := oidcRound(t, nobody, idp, nil, nil); !strings.Contains(to, "err=sso_") || session != "" {
		t.Errorf("link by an unverified email = %s session %q", to, session)
	}

	// Verified, bob's account opens, the administrator's never does.
	idp.verify = true
	if to, session := oidcRound(t, nobody, idp, nil, nil); to != "/app/" || session == "" {
		t.Errorf("link by email for a member = %s session %q", to, session)
	}

	idp.email = "alice@corp.example"
	if to, session := oidcRound(t, nobody, idp, nil, nil); !strings.Contains(to, "err=sso_link_required") || session != "" {
		t.Errorf("link by email for an administrator = %s session %q", to, session)
	}

	if n := accounts(); n != 4 {
		t.Errorf("accounts = %d, want 4", n)
	}

	var mine struct {
		Identities []struct {
			ID           string `json:"id"`
			ProviderSlug string `json:"provider_slug"`
			Email        string `json:"email"`
		} `json:"identities"`
	}

	bob.expect(http.StatusOK, "GET", "/api/auth/identities", nil, &mine)
	if len(mine.Identities) != 1 || mine.Identities[0].ProviderSlug != "corp-idp" {
		t.Errorf("bob's identities = %+v", mine.Identities)
	}

	// The administrator links the provider signed in, under another
	// address at the IdP, and signs in through it afterwards.
	idp.email = "alice.work@corp.example"
	if to, _ := oidcRound(t, nobody, idp, alice, alice); to != "/app/profile?linked=corp-idp" {
		t.Fatalf("link = %s", to)
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/identities", nil, &mine)
	if len(mine.Identities) != 1 || mine.Identities[0].Email != "alice.work@corp.example" {
		t.Errorf("alice's identities = %+v", mine.Identities)
	}

	if to, session := oidcRound(t, nobody, idp, nil, nil); to != "/app/" || session == "" {
		t.Errorf("sign-in through the linked identity = %s session %q", to, session)
	}

	// A link finished in another person's session links nothing.
	idp.email = "carol.work@corp.example"
	if to, _ := oidcRound(t, nobody, idp, carol, dave); to != "/app/profile?err=sso_link_session" {
		t.Errorf("carol's link finished in dave's session = %s", to)
	}

	dave.expect(http.StatusOK, "GET", "/api/auth/identities", nil, &mine)
	if len(mine.Identities) != 0 {
		t.Errorf("dave's identities = %+v", mine.Identities)
	}

	// An identity bob holds is not dave's to take.
	idp.email = "bob@corp.example"
	if to, _ := oidcRound(t, nobody, idp, dave, dave); to != "/app/profile?err=sso_identity_taken" {
		t.Errorf("linking bob's identity to dave = %s", to)
	}

	// carol has a password, so her only identity unlinks, and only by
	// her. An account the provider made has no other way in.
	idp.email = "carol.work@corp.example"
	if to, _ := oidcRound(t, nobody, idp, carol, carol); to != "/app/profile?linked=corp-idp" {
		t.Fatalf("carol's link = %s", to)
	}

	carol.expect(http.StatusOK, "GET", "/api/auth/identities", nil, &mine)
	bob.expect(http.StatusNotFound, "DELETE", "/api/auth/identities/"+mine.Identities[0].ID, nil)
	carol.expect(http.StatusNoContent, "DELETE", "/api/auth/identities/"+mine.Identities[0].ID, nil)

	idp.email = "erin@corp.example"
	_, session := oidcRound(t, nobody, idp, nil, nil)
	erin := &api{t: t, srv: srv, cookie: session}
	erin.expect(http.StatusOK, "GET", "/api/auth/identities", nil, &mine)
	if len(mine.Identities) != 1 {
		t.Fatalf("erin's identities = %+v", mine.Identities)
	}

	erin.expect(http.StatusConflict, "DELETE", "/api/auth/identities/"+mine.Identities[0].ID, nil)
}
