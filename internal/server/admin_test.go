// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/cron"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// The admin panel is the first account's: a second account gets 403 on
// every admin route, the last enabled administrator cannot be demoted,
// disabled or deleted, a reset link the admin hands out works, and an
// account owning an investigation is not deleted.
func TestTheAdminPanelKeepsAnAdministrator(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	for _, r := range [][2]string{{"GET", "/api/admin/users"}, {"POST", "/api/admin/users"}, {"GET", "/api/admin/users/x"}} {
		if code := bob.do(r[0], r[1], map[string]any{}, nil); code != http.StatusForbidden {
			t.Errorf("%s %s by a member = %d, want 403", r[0], r[1], code)
		}
	}

	var list struct {
		Users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Admin bool   `json:"admin"`
		} `json:"users"`
		Total int `json:"total"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/users?q=BOB", nil, &list)
	if list.Total != 1 || len(list.Users) != 1 || list.Users[0].Email != "bob@example.com" || list.Users[0].Admin {
		t.Fatalf("list = %+v", list)
	}

	bobID := list.Users[0].ID
	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)

	// Alice cannot touch her own flags or delete herself.
	alice.expect(http.StatusForbidden, "PATCH", "/api/admin/users/"+me.User.ID, map[string]any{"admin": false})
	alice.expect(http.StatusForbidden, "DELETE", "/api/admin/users/"+me.User.ID, nil)

	// Nor set her own password or mint herself a reset link, which would
	// skip the current password the profile asks for.
	alice.expect(http.StatusForbidden, "PATCH", "/api/admin/users/"+me.User.ID, map[string]any{"password": "another horse battery"})
	alice.expect(http.StatusForbidden, "POST", "/api/admin/users/"+me.User.ID+"/reset-link", nil)

	// Promoted, Bob can demote Alice, then she is out of the panel. He
	// cannot demote himself, so the install keeps an administrator.
	alice.expect(http.StatusOK, "PATCH", "/api/admin/users/"+bobID, map[string]any{"admin": true})
	bob.expect(http.StatusOK, "PATCH", "/api/admin/users/"+me.User.ID, map[string]any{"admin": false})
	alice.expect(http.StatusForbidden, "GET", "/api/admin/users", nil)
	bob.expect(http.StatusForbidden, "PATCH", "/api/admin/users/"+bobID, map[string]any{"admin": false})
	bob.expect(http.StatusOK, "PATCH", "/api/admin/users/"+me.User.ID, map[string]any{"admin": true})

	// Disabling ends the sessions at once and the account cannot sign in.
	alice.expect(http.StatusOK, "PATCH", "/api/admin/users/"+bobID, map[string]any{"disabled": true})
	bob.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	bob.expect(http.StatusUnauthorized, "POST", "/api/auth/login", map[string]string{"email": "bob@example.com", "password": "correct horse"})
	alice.expect(http.StatusOK, "PATCH", "/api/admin/users/"+bobID, map[string]any{"disabled": false})
	bob.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "bob@example.com", "password": "correct horse"})

	// A password set by the admin signs the person out everywhere and the
	// new one works.
	alice.expect(http.StatusOK, "PATCH", "/api/admin/users/"+bobID, map[string]any{"password": "set by alice"})
	bob.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	bob.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "bob@example.com", "password": "set by alice"})

	// Second factor and passkeys are reset by the admin, not on herself.
	alice.expect(http.StatusForbidden, "DELETE", "/api/admin/users/"+me.User.ID+"/2fa", nil)
	alice.expect(http.StatusNoContent, "DELETE", "/api/admin/users/"+bobID+"/2fa", nil)
	alice.expect(http.StatusOK, "DELETE", "/api/admin/users/"+bobID+"/passkeys", nil)
	alice.expect(http.StatusOK, "POST", "/api/admin/users/"+bobID+"/revoke-sessions", nil)
	bob.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)

	// Created by the admin without a password: a local account waiting
	// for a reset link, listed and deletable.
	var made struct {
		User struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/admin/users", map[string]any{"email": "carol@example.com"}, &made)
	alice.expect(http.StatusConflict, "POST", "/api/admin/users", map[string]any{"email": "carol@example.com"})
	alice.expect(http.StatusNoContent, "DELETE", "/api/admin/users/"+made.User.ID, nil)
	alice.expect(http.StatusNotFound, "GET", "/api/admin/users/"+made.User.ID, nil)
}

// A reset link from the admin panel signs the person in with a new
// password, and an account owning an investigation is refused deletion
// until it is handed over.
func TestAdminResetLinkAndOwnershipTransfer(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t), func(cfg *env.Config) { cfg.Server.PublicURL = "https://gosint.example" })
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")
	inv, _ := investigationFor(t, bob)

	var list struct {
		Users []struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"users"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/users?q=bob", nil, &list)
	bobID := list.Users[0].ID

	var link struct {
		Link    string `json:"link"`
		Emailed bool   `json:"emailed"`
	}

	alice.expect(http.StatusOK, "POST", "/api/admin/users/"+bobID+"/reset-link", nil, &link)
	m := tokenRe.FindStringSubmatch(link.Link)
	if m == nil || !strings.HasPrefix(link.Link, "https://gosint.example/app/reset-password?token=") || !link.Emailed {
		t.Fatalf("reset link = %+v", link)
	}

	nobody := &api{t: t, srv: srv}
	nobody.expect(http.StatusOK, "POST", "/api/auth/password-reset/confirm", map[string]string{"token": m[1], "password": "bob has a new one"}) //nolint:gosec // a test password
	bob.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	bob.expect(http.StatusOK, "POST", "/api/auth/login", map[string]string{"email": "bob@example.com", "password": "bob has a new one"}) //nolint:gosec // a test password

	// Bob owns an investigation: deletion is refused until he hands it to
	// a member.
	code, body, _ := alice.raw("DELETE", "/api/admin/users/"+bobID, nil)
	if code != http.StatusConflict || !strings.Contains(body, "Case") {
		t.Fatalf("delete an owner = %d %s", code, body)
	}

	var me struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
	bob.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "alice@example.com", "role": "viewer"})
	bob.expect(http.StatusNoContent, "PUT", inv+"/owner", map[string]string{"user_id": me.User.ID})

	var got struct {
		Investigation struct {
			OwnerID string `json:"owner_id"`
			Role    string `json:"current_user_role"`
		} `json:"investigation"`
	}

	alice.expect(http.StatusOK, "GET", inv, nil, &got)
	if got.Investigation.OwnerID != me.User.ID || got.Investigation.Role != "owner" {
		t.Errorf("after the transfer = %+v", got.Investigation)
	}

	bob.expect(http.StatusOK, "GET", inv, nil, &got)
	if got.Investigation.Role != "admin" {
		t.Errorf("the old owner's role = %s, want admin", got.Investigation.Role)
	}

	alice.expect(http.StatusNoContent, "DELETE", "/api/admin/users/"+bobID, nil)
	bob.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
}

// investigationFor makes an investigation owned by a, returning its API
// path and id.
func investigationFor(t *testing.T, a *api) (string, string) {
	t.Helper()

	var created struct {
		Investigation idBody `json:"investigation"`
	}

	a.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &created)

	return "/api/investigations/" + created.Investigation.ID, created.Investigation.ID
}

// An administrator declares an entity type: it lists with the built-in
// ones at once, takes entities like any type, cannot be deleted while
// one exists, and a plain member cannot declare one.
func TestAnAdministratorDeclaresAType(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	admin := signUp(t, srv, "admin@example.com")
	bob := signUp(t, srv, "bob@example.com")
	t.Cleanup(func() { _ = types.Builtin().SetCustom(nil) })

	def := map[string]any{"name": "badge", "title": "Badge", "category": "person", "icon": "Tag", "color": "#336699",
		"fields": []map[string]any{{"name": "number", "title": "Number", "kind": "string", "primary": true}, {"name": "level", "kind": "int"}}}
	bob.expect(http.StatusForbidden, "POST", "/api/admin/types", def)

	var made struct {
		Type struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"type"`
	}

	admin.expect(http.StatusCreated, "POST", "/api/admin/types", def, &made)
	admin.expect(http.StatusConflict, "POST", "/api/admin/types", def)
	admin.expect(http.StatusBadRequest, "POST", "/api/admin/types", map[string]any{"name": "domain", "title": "x", "category": "global",
		"fields": []map[string]any{{"name": "a", "kind": "string", "primary": true}}})

	var list struct {
		Types []struct {
			Name string `json:"name"`
		} `json:"types"`
	}

	bob.expect(http.StatusOK, "GET", "/api/types", nil, &list)
	found := false
	for _, x := range list.Types {
		found = found || x.Name == "badge"
	}

	if !found {
		t.Fatal("the catalog lacks the custom type")
	}

	sketch, _ := sketchFor(t, bob)

	var n struct {
		Node struct {
			ID         string         `json:"id"`
			Label      string         `json:"label"`
			Properties map[string]any `json:"properties"`
		} `json:"node"`
	}

	bob.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "badge", "label": "B-17", "properties": map[string]any{"level": "2"}}, &n)
	if n.Node.Label != "B-17" || n.Node.Properties["number"] != "B-17" {
		t.Errorf("node = %+v", n.Node)
	}

	bob.expect(http.StatusBadRequest, "POST", sketch+"/nodes", map[string]any{"type": "badge", "label": "B-18", "properties": map[string]any{"level": "two"}})

	admin.expect(http.StatusConflict, "DELETE", "/api/admin/types/"+made.Type.ID, nil)
	bob.expect(http.StatusOK, "POST", sketch+"/nodes/delete", map[string]any{"ids": []string{n.Node.ID}})
	admin.expect(http.StatusNoContent, "DELETE", "/api/admin/types/"+made.Type.ID, nil)
	bob.expect(http.StatusBadRequest, "POST", sketch+"/nodes", map[string]any{"type": "badge", "label": "B-19"})
}

// A job that fails answers a generic message, its error stays in the
// server log and the job list.
func TestAFailedJobKeepsItsErrorOffTheWire(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	rt.Cron.Register(cron.Job{Name: "leaky", Schedule: cron.EveryInterval(time.Hour),
		Run: func(context.Context) error { return errors.New("open /srv/secret/bucket: denied") }})

	alice := signUp(t, srv, "alice@example.com")
	code, body, _ := alice.raw("POST", "/api/admin/jobs/leaky/run", nil)
	if code != http.StatusBadGateway {
		t.Fatalf("run = %d, want 502", code)
	}

	if strings.Contains(body, "/srv/secret") {
		t.Errorf("the job's raw error reached the wire: %s", body)
	}

	var list struct {
		Jobs []struct {
			Name      string `json:"name"`
			LastError string `json:"last_error"`
		} `json:"jobs"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/jobs", nil, &list)
	if len(list.Jobs) != 1 || !strings.Contains(list.Jobs[0].LastError, "denied") {
		t.Errorf("jobs = %+v, want the error in last_error", list.Jobs)
	}
}
