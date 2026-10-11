// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// An invitation admits a new account while registration is closed: the
// link says what it is for, the register call carries the token, accept
// makes a member with the invited role, the token is spent, and a
// stranger without a token is still refused.
func TestAnInvitationAdmitsANewAccountWhileRegistrationIsClosed(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t), func(cfg *env.Config) {
		cfg.Auth.OpenRegistration = false
		cfg.Server.PublicURL = "https://gosint.example"
	})
	alice := signUp(t, srv, "alice@example.com")
	inv, invID := investigationFor(t, alice)

	nobody := &api{t: t, srv: srv}
	nobody.expect(http.StatusForbidden, "POST", "/api/auth/register", map[string]string{"email": "bob@example.com", "password": "correct horse"})

	var created struct {
		Invitation struct {
			ID    string `json:"id"`
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"invitation"`
		Link    string `json:"link"`
		Emailed bool   `json:"emailed"`
	}

	alice.expect(http.StatusCreated, "POST", inv+"/invitations", map[string]string{"email": "bob@example.com", "role": "editor"}, &created)
	token, ok := strings.CutPrefix(created.Link, "https://gosint.example/app/invitations/")
	if !ok || len(token) != 64 || !created.Emailed || created.Invitation.Role != "editor" {
		t.Fatalf("created = %+v", created)
	}

	if sent := rt.Mail.(*mailer.Fake).Messages(); len(sent) != 1 || !strings.Contains(sent[0].Text, created.Link) {
		t.Errorf("mail = %+v, want the link to bob", sent)
	}

	var pending struct {
		Invitations []struct {
			Email string `json:"email"`
		} `json:"invitations"`
	}

	alice.expect(http.StatusOK, "GET", inv+"/invitations", nil, &pending)
	if len(pending.Invitations) != 1 {
		t.Errorf("pending = %+v", pending)
	}

	var look struct {
		Investigation string `json:"investigation"`
		Email         string `json:"email"`
		HasAccount    bool   `json:"has_account"`
		SignedInAs    string `json:"signed_in_as"`
	}

	nobody.expect(http.StatusOK, "GET", "/api/invitations/"+token, nil, &look)
	if look.Investigation != "Case" || look.Email != "bob@example.com" || look.HasAccount || look.SignedInAs != "" {
		t.Errorf("lookup = %+v", look)
	}

	alice.expect(http.StatusOK, "GET", "/api/invitations/"+token, nil, &look)
	if look.SignedInAs != "alice@example.com" {
		t.Errorf("lookup signed in = %+v", look)
	}

	// The wrong address cannot register on it, the right one can.
	nobody.expect(http.StatusForbidden, "POST", "/api/auth/register",
		map[string]string{"email": "carol@example.com", "password": "correct horse", "invite": token})
	bob := &api{t: t, srv: srv}
	bob.expect(http.StatusCreated, "POST", "/api/auth/register",
		map[string]string{"email": "bob@example.com", "password": "correct horse", "invite": token})

	// Alice signed in cannot take bob's invitation.
	alice.expect(http.StatusForbidden, "POST", "/api/invitations/"+token+"/accept", nil)

	var accepted struct {
		InvestigationID string `json:"investigation_id"`
		Member          struct {
			Role string `json:"role"`
		} `json:"member"`
	}

	bob.expect(http.StatusOK, "POST", "/api/invitations/"+token+"/accept", nil, &accepted)
	if accepted.InvestigationID != invID || accepted.Member.Role != "editor" {
		t.Errorf("accepted = %+v", accepted)
	}

	bob.expect(http.StatusOK, "GET", inv, nil)
	nobody.expect(http.StatusNotFound, "GET", "/api/invitations/"+token, nil)
	bob.expect(http.StatusNotFound, "POST", "/api/invitations/"+token+"/accept", nil)

	// An address with an account that is already a member needs none,
	// a revoked one is gone.
	alice.expect(http.StatusConflict, "POST", inv+"/invitations", map[string]string{"email": "bob@example.com", "role": "viewer"})
	alice.expect(http.StatusCreated, "POST", inv+"/invitations", map[string]string{"email": "dave@example.com", "role": "viewer"}, &created)
	alice.expect(http.StatusNoContent, "DELETE", inv+"/invitations/"+created.Invitation.ID, nil)
	alice.expect(http.StatusNotFound, "GET", "/api/invitations/"+strings.TrimPrefix(created.Link, "https://gosint.example/app/invitations/"), nil)
}
