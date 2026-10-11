// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// Behind a listed proxy the forwarded address is the client's, so the
// sign-in limiter and the session records see people, not the proxy.
// From anyone else the header is ignored.
func TestAListedProxyForwardsTheClientAddress(t *testing.T) {
	clientOf := func(t *testing.T, tune func(*env.Config), forwarded ...string) string {
		t.Helper()

		header := "203.0.113.7"
		if len(forwarded) > 0 {
			header = forwarded[0]
		}

		srv := newAPI(t, dbtest.SQLite(t), tune)
		a := &api{t: t, srv: srv}
		if code := a.do("POST", "/api/auth/register", map[string]string{"email": "alice@example.com", "password": "correct horse"},
			nil, "X-Forwarded-For", header); code != http.StatusCreated {
			t.Fatalf("register = %d", code)
		}

		var list struct {
			Sessions []struct {
				ClientIP string `json:"client_ip"`
			} `json:"sessions"`
		}

		a.expect(http.StatusOK, "GET", "/api/auth/sessions", nil, &list)
		if len(list.Sessions) != 1 {
			t.Fatalf("sessions = %+v", list.Sessions)
		}

		return list.Sessions[0].ClientIP
	}

	if ip := clientOf(t, func(cfg *env.Config) { cfg.Server.TrustedProxies = []string{"0.0.0.0/0"} }); ip != "203.0.113.7" {
		t.Errorf("behind a listed proxy the client is %q, want the forwarded address", ip)
	}

	if ip := clientOf(t, func(*env.Config) {}); ip == "203.0.113.7" {
		t.Error("the forwarded address was read from an unlisted peer")
	}

	// A proxy appends the address it saw to what the client sent: the
	// client is the rightmost address no listed proxy holds, whatever
	// the client wrote before it.
	onlyThePeer := func(cfg *env.Config) { cfg.Server.TrustedProxies = []string{"0.0.0.0/32"} }
	for _, forged := range []string{"1.2.3.4, 198.51.100.9", "not-an-address, 198.51.100.9", "10.0.0.1,198.51.100.9"} {
		if ip := clientOf(t, onlyThePeer, forged); ip != "198.51.100.9" {
			t.Errorf("X-Forwarded-For %q gave client %q, want the address the proxy appended", forged, ip)
		}
	}
}
