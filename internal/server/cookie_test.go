// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// TLS ended at a proxy the server was not told about still gives a
// Secure session cookie when the public URL is https.
func TestAnHTTPSPublicURLMakesCookiesSecure(t *testing.T) {
	sessionCookie := func(t *testing.T, publicURL string) *http.Cookie {
		t.Helper()

		srv := newAPI(t, dbtest.SQLite(t), func(cfg *env.Config) { cfg.Server.PublicURL = publicURL })
		a := &api{t: t, srv: srv}
		code, _, headers := a.raw("POST", "/api/auth/register", map[string]string{"email": "alice@example.com", "password": "correct horse"})
		if code != http.StatusCreated {
			t.Fatalf("register = %d", code)
		}

		for _, line := range headers.Values("Set-Cookie") {
			if strings.HasPrefix(line, auth.CookieName+"=") {
				ck, err := http.ParseSetCookie(line)
				if err != nil {
					t.Fatal(err)
				}

				return ck
			}
		}

		t.Fatal("no session cookie")

		return nil
	}

	if ck := sessionCookie(t, "https://osint.example.com"); !ck.Secure {
		t.Error("an https public URL gave a cookie without Secure")
	}

	if ck := sessionCookie(t, "http://localhost:5001"); ck.Secure {
		t.Error("a plain http install got a Secure cookie it could not send back")
	}
}
