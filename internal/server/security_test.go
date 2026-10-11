// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// rawWith sends a request with the given cookies and headers, no
// session of its own, and hands back the status, body and headers.
func rawWith(t *testing.T, a *api, method, path string, body any, cookies map[string]string, headers ...string) (int, string, http.Header) {
	t.Helper()

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}

		r = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	for name, v := range cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: v}) //nolint:gosec // a test request, not a response
	}

	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	resp, err := a.srv.App().Test(req, testConfig)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	out, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(out), resp.Header
}

// cookieNamed is the value a response set for name, "" for none.
func cookieNamed(hdr http.Header, name string) string {
	for _, line := range hdr.Values("Set-Cookie") {
		if strings.HasPrefix(line, name+"=") {
			v, _, _ := strings.Cut(strings.TrimPrefix(line, name+"="), ";")

			return v
		}
	}

	return ""
}

var tokenRe = regexp.MustCompile(`token=([0-9a-f]{64})`)

// The first account is the administrator, the sign-in page learns what
// the install offers, and a reset link read from the fake mailer sets
// a new password, ends the old session and works once.
func TestPasswordResetRoundTrip(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t), func(cfg *env.Config) {
		cfg.Server.PublicURL = "https://gosint.example"
	})
	fake := rt.Mail.(*mailer.Fake)
	alice := signUp(t, srv, "alice@example.com")

	var me struct {
		User struct {
			Admin       bool `json:"admin"`
			AccountType int  `json:"account_type"`
		} `json:"user"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
	if !me.User.Admin || me.User.AccountType != 1 {
		t.Errorf("the first account = %+v, want a local admin", me.User)
	}

	var info struct {
		MailEnabled      bool `json:"mail_enabled"`
		RegistrationOpen bool `json:"registration_open"`
		PasskeysEnabled  bool `json:"passkeys_enabled"`
	}

	nobody := &api{t: t, srv: srv}
	nobody.expect(http.StatusOK, "GET", "/api/auth/info", nil, &info)
	if !info.MailEnabled || !info.RegistrationOpen {
		t.Errorf("info = %+v", info)
	}

	// The same answer for a stranger's address and for a real one.
	var msg struct {
		Message string `json:"message"`
	}

	nobody.expect(http.StatusOK, "POST", "/api/auth/password-reset/request", map[string]string{"email": "nobody@example.com"}, &msg)
	nobody.expect(http.StatusOK, "POST", "/api/auth/password-reset/request", map[string]string{"email": "alice@example.com"}, &msg)

	deadline := time.Now().Add(5 * time.Second)
	for len(fake.Messages()) < 1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	sent := fake.Messages()
	if len(sent) != 1 || sent[0].To[0] != "alice@example.com" {
		t.Fatalf("mail sent = %+v, want one to alice", sent)
	}

	m := tokenRe.FindStringSubmatch(sent[0].Text)
	if m == nil || !strings.Contains(sent[0].Text, "https://gosint.example/app/reset-password?token=") {
		t.Fatalf("no reset link in %q", sent[0].Text)
	}

	nobody.expect(http.StatusBadRequest, "POST", "/api/auth/password-reset/confirm",
		map[string]string{"token": strings.Repeat("0", 64), "password": "new horse battery"})
	nobody.expect(http.StatusOK, "POST", "/api/auth/password-reset/confirm",
		map[string]string{"token": m[1], "password": "new horse battery"})
	nobody.expect(http.StatusBadRequest, "POST", "/api/auth/password-reset/confirm",
		map[string]string{"token": m[1], "password": "new horse battery"})

	// The old session is gone, the new password signs in.
	alice.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	alice.expect(http.StatusUnauthorized, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})
	alice.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "new horse battery"})

	// Changing it signed in re-checks the current one and keeps this session.
	alice.expect(http.StatusForbidden, "POST", "/api/auth/password",
		map[string]string{"current_password": "wrong", "password": "another horse"})
	alice.expect(http.StatusOK, "POST", "/api/auth/password",
		map[string]string{"current_password": "new horse battery", "password": "another horse"})
	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)
}

// The authenticator: setup is behind the password, a code turns it on
// and hands out recovery codes, sign-in becomes two steps, a code works
// once and a recovery code once, and enabling ended the other session.
func TestTOTPLoginIsTwoSteps(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	other := &api{t: t, srv: srv}
	other.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})

	alice.expect(http.StatusForbidden, "POST", "/api/auth/2fa/setup", map[string]string{"password": "wrong"})

	var setup struct {
		Secret     string `json:"secret"`
		OTPAuthURL string `json:"otpauth_url"`
	}

	alice.expect(http.StatusOK, "POST", "/api/auth/2fa/setup", map[string]string{"password": "correct horse"}, &setup)
	if setup.Secret == "" || !strings.Contains(setup.OTPAuthURL, "otpauth://totp/Gosint:alice@example.com") {
		t.Fatalf("setup = %+v", setup)
	}

	code := func() string {
		c, err := totp.GenerateCode(setup.Secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}

		return c
	}

	alice.expect(http.StatusBadRequest, "POST", "/api/auth/2fa/enable", map[string]string{"code": "000000"})

	var enabled struct {
		TOTPEnabled   bool     `json:"totp_enabled"`
		RecoveryCodes []string `json:"recovery_codes"`
	}

	alice.expect(http.StatusOK, "POST", "/api/auth/2fa/enable", map[string]string{"code": code()}, &enabled)
	if !enabled.TOTPEnabled || len(enabled.RecoveryCodes) != 10 {
		t.Fatalf("enable = %+v", enabled)
	}

	// The other session ended, this one stayed.
	other.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)

	// Sign in: the password answers no session but the factor owed.
	fresh := &api{t: t, srv: srv}
	codeStep, body, hdr := rawWith(t, fresh, "POST", "/api/auth/login",
		map[string]string{"email": "alice@example.com", "password": "correct horse"}, nil)
	pending := cookieNamed(hdr, "gosint_2fa")
	if codeStep != http.StatusOK || !strings.Contains(body, `"second_factor":"totp"`) || strings.Contains(body, `"user"`) || pending == "" {
		t.Fatalf("first step = %d %s cookie %q", codeStep, body, pending)
	}

	if cookieNamed(hdr, auth.CookieName) != "" {
		t.Fatal("a session was opened before the code")
	}

	// A code the enable step already spent is refused, the next step works
	// once, a replay is refused.
	status, _, _ := rawWith(t, fresh, "POST", "/api/auth/login/2fa", map[string]string{"code": "123456"}, map[string]string{"gosint_2fa": pending})
	if status != http.StatusUnauthorized {
		t.Errorf("wrong code = %d, want 401", status)
	}

	var ok int
	var sessionCookie string
	for range 3 {
		ok, body, hdr = rawWith(t, fresh, "POST", "/api/auth/login/2fa", map[string]string{"code": code()}, map[string]string{"gosint_2fa": pending})
		if ok == http.StatusOK {
			sessionCookie = cookieNamed(hdr, auth.CookieName)

			break
		}

		// The code of the step enable spent is still current, wait for the next one.
		time.Sleep(time.Until(time.Unix((totpStep(time.Now())+1)*30, 0)) + 100*time.Millisecond)
	}

	if ok != http.StatusOK || sessionCookie == "" || !strings.Contains(body, `"user"`) {
		t.Fatalf("second step = %d %s", ok, body)
	}

	replay, _, _ := rawWith(t, fresh, "POST", "/api/auth/login/2fa", map[string]string{"code": code()}, map[string]string{"gosint_2fa": pending})
	if replay != http.StatusUnauthorized {
		t.Errorf("a replayed code = %d, want 401", replay)
	}

	// A recovery code signs in once.
	_, _, hdr = rawWith(t, fresh, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"}, nil)
	pending = cookieNamed(hdr, "gosint_2fa")
	rec := enabled.RecoveryCodes[0]
	if status, _, _ := rawWith(t, fresh, "POST", "/api/auth/login/2fa", map[string]string{"code": strings.ToUpper(rec)}, map[string]string{"gosint_2fa": pending}); status != http.StatusOK {
		t.Errorf("recovery code = %d, want 200", status)
	}

	_, _, hdr = rawWith(t, fresh, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"}, nil)
	pending = cookieNamed(hdr, "gosint_2fa")
	if status, _, _ := rawWith(t, fresh, "POST", "/api/auth/login/2fa", map[string]string{"code": rec}, map[string]string{"gosint_2fa": pending}); status != http.StatusUnauthorized {
		t.Errorf("a spent recovery code = %d, want 401", status)
	}

	var remaining struct {
		Remaining int `json:"remaining"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/2fa/recovery-codes", nil, &remaining)
	if remaining.Remaining != 9 {
		t.Errorf("remaining = %d, want 9", remaining.Remaining)
	}

	// Turning it off takes a current code and drops the codes.
	alice.expect(http.StatusBadRequest, "POST", "/api/auth/2fa/disable", map[string]string{"code": "000000"})
	time.Sleep(time.Until(time.Unix((totpStep(time.Now())+1)*30, 0)) + 100*time.Millisecond)
	alice.expect(http.StatusOK, "POST", "/api/auth/2fa/disable", map[string]string{"code": code()})
	alice.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})
}

func totpStep(at time.Time) int64 { return at.Unix() / 30 }

// Five wrong passwords lock the account for a while, and a right one
// is then refused too.
func TestLoginLocksAfterRepeatedFailures(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	signUp(t, srv, "alice@example.com")
	guest := &api{t: t, srv: srv}
	for range 5 {
		guest.expect(http.StatusUnauthorized, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "wrong"})
	}

	guest.expect(http.StatusUnauthorized, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})
}

// A passkey ceremony needs a browser Origin, seals its state in a cookie
// and is answerable once.
func TestPasskeyCeremoniesNeedAnOriginAndAreSingleUse(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	if code := alice.do("POST", "/api/auth/passkeys/login/begin", nil, nil); code != http.StatusBadRequest {
		t.Errorf("begin without an Origin = %d, want 400", code)
	}

	status, body, hdr := rawWith(t, alice, "POST", "/api/auth/passkeys/login/begin", nil, nil, "Origin", "http://example.com")
	ceremony := cookieNamed(hdr, "gosint_passkey_login")
	if status != http.StatusOK || !strings.Contains(body, `"challenge"`) || ceremony == "" {
		t.Fatalf("begin = %d %s cookie %q", status, body, ceremony)
	}

	first, msg1, _ := rawWith(t, alice, "POST", "/api/auth/passkeys/login/finish", map[string]any{"id": "x"},
		map[string]string{"gosint_passkey_login": ceremony}, "Origin", "http://example.com")
	second, msg2, _ := rawWith(t, alice, "POST", "/api/auth/passkeys/login/finish", map[string]any{"id": "x"},
		map[string]string{"gosint_passkey_login": ceremony}, "Origin", "http://example.com")
	if first != http.StatusUnauthorized || second != http.StatusBadRequest || !strings.Contains(msg2, "expired") {
		t.Errorf("finish twice = %d %s then %d %s, want a refused assertion then a spent ceremony", first, msg1, second, msg2)
	}

	// Enrolment is behind the password, and off when the install says so.
	alice.expect(http.StatusForbidden, "POST", "/api/auth/passkeys/register/begin", map[string]string{"password": "wrong"}, nil)
	var list struct {
		Passkeys []any `json:"passkeys"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/passkeys", nil, &list)
	if len(list.Passkeys) != 0 {
		t.Errorf("passkeys = %v, want none", list.Passkeys)
	}

	off := newAPI(t, dbtest.SQLite(t), func(cfg *env.Config) { cfg.Auth.PasskeysEnabled = false })
	bob := signUp(t, off, "bob@example.com")
	bob.expect(http.StatusForbidden, "GET", "/api/auth/passkeys", nil)
	bob.expect(http.StatusBadRequest, "POST", "/api/auth/passkeys/login/begin", nil)
}

// The sessions list marks this one, another can be ended, and ending
// the others keeps this one.
func TestSessionsListAndRevoke(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	phone := &api{t: t, srv: srv}
	phone.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})

	var list struct {
		Sessions []struct {
			ID      string `json:"id"`
			Current bool   `json:"current"`
		} `json:"sessions"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/sessions", nil, &list)
	if len(list.Sessions) != 2 {
		t.Fatalf("sessions = %+v, want 2", list.Sessions)
	}

	var otherID string
	current := 0
	for _, s := range list.Sessions {
		if s.Current {
			current++
		} else {
			otherID = s.ID
		}
	}

	if current != 1 || otherID == "" {
		t.Fatalf("sessions = %+v, want one current", list.Sessions)
	}

	alice.expect(http.StatusOK, "DELETE", "/api/auth/sessions/"+otherID, nil)
	phone.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	alice.expect(http.StatusNotFound, "DELETE", "/api/auth/sessions/"+otherID, nil)

	phone.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "correct horse"})

	var revoked struct {
		Revoked int `json:"revoked"`
	}

	alice.expect(http.StatusOK, "POST", "/api/auth/sessions/revoke-others", nil, &revoked)
	if revoked.Revoked != 1 {
		t.Errorf("revoked = %d, want 1", revoked.Revoked)
	}

	phone.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)
}
