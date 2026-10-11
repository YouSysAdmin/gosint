// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"testing"

	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

type sealStatus struct {
	Initialized bool `json:"initialized"`
	Sealed      bool `json:"sealed"`
}

// A sealed server answers nothing but the seal routes, and opens with
// the unseal key it was initialized with.
func TestTheServerStartsSealed(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	rt.Vault.Reseal()

	admin := &api{t: t, srv: srv}
	admin.expect(http.StatusServiceUnavailable, "GET", "/api/auth/info", nil)

	var st sealStatus
	admin.expect(http.StatusOK, "GET", "/api/sys/seal-status", nil, &st)
	if st.Initialized || !st.Sealed {
		t.Fatalf("fresh status = %+v", st)
	}

	const unsealKey = "a long enough unseal key"
	admin.expect(http.StatusBadRequest, "POST", "/api/sys/init", map[string]string{"unseal_key": "short"})
	admin.expect(http.StatusOK, "POST", "/api/sys/init", map[string]string{"unseal_key": unsealKey}, &st)
	if !st.Initialized || st.Sealed {
		t.Fatalf("after init = %+v", st)
	}

	admin.expect(http.StatusConflict, "POST", "/api/sys/init", map[string]string{"unseal_key": unsealKey})
	admin = signUp(t, srv, "admin@example.com")
	admin.expect(http.StatusOK, "GET", "/api/auth/me", nil)

	stranger := &api{t: t, srv: srv}
	stranger.expect(http.StatusUnauthorized, "POST", "/api/admin/seal", nil)

	admin.expect(http.StatusOK, "POST", "/api/admin/seal", nil, &st)
	if !st.Sealed {
		t.Fatalf("after seal = %+v", st)
	}

	admin.expect(http.StatusServiceUnavailable, "GET", "/api/auth/me", nil)
	admin.expect(http.StatusBadRequest, "POST", "/api/sys/unseal", map[string]string{"unseal_key": "not the unseal key"})
	admin.expect(http.StatusOK, "POST", "/api/sys/unseal", map[string]string{"unseal_key": unsealKey}, &st)
	if st.Sealed {
		t.Fatalf("after unseal = %+v", st)
	}

	admin.expect(http.StatusOK, "GET", "/api/auth/me", nil)
}

// Accounts sealed under a key the database no longer wraps: a fresh key
// would leave them unread, so initializing is refused.
func TestAnInitOverSealedDataIsRefused(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	admin := signUp(t, srv, "admin@example.com")
	rt.Vault.Reseal()

	admin.expect(http.StatusConflict, "POST", "/api/sys/init", map[string]string{"unseal_key": "a long enough unseal key"})
}
