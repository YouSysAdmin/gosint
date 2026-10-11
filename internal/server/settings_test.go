// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"encoding/json/v2"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// The models are the administrator's to offer, the first one is the
// default, the default moves by naming another, deleting it hands the
// switch on, and a person sees which secret a model wants but never
// the server's key.
func TestAnAdministratorOffersModels(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	type pick struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Default  bool   `json:"default"`
		KeyName  string `json:"key_name"`
		NeedsKey bool   `json:"needs_key"`
	}

	var picks struct {
		Models []pick `json:"models"`
	}

	bob.expect(http.StatusOK, "GET", "/api/models", nil, &picks)
	if len(picks.Models) != 0 {
		t.Fatalf("a fresh install offers %+v", picks.Models)
	}

	bob.expect(http.StatusForbidden, "POST", "/api/admin/models", map[string]any{"name": "x", "base_url": "http://localhost:1234/v1"})
	alice.expect(http.StatusBadRequest, "POST", "/api/admin/models", map[string]any{"name": "Remote", "base_url": "https://api.example.com/v1"})

	var local, remote struct {
		Model struct {
			ID      string `json:"id"`
			Default bool   `json:"default"`
			HasKey  bool   `json:"has_api_key"`
		} `json:"model"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/admin/models", map[string]any{"name": "Local", "base_url": "http://localhost:1234/v1"}, &local)
	code, body, _ := alice.raw("POST", "/api/admin/models", map[string]any{"name": "Remote", "base_url": "https://api.example.com/v1",
		"model": "big", "key_name": "REMOTE_KEY", "api_key": "server-secret"})
	if code != http.StatusCreated || strings.Contains(body, "server-secret") {
		t.Fatalf("create = %d %s", code, body)
	}

	if err := json.Unmarshal([]byte(body), &remote); err != nil {
		t.Fatal(err)
	}

	if !local.Model.Default || remote.Model.Default || !remote.Model.HasKey {
		t.Fatalf("local %+v remote %+v", local.Model, remote.Model)
	}

	alice.expect(http.StatusOK, "PATCH", "/api/admin/models/"+remote.Model.ID, map[string]any{"default": true})
	code, body, _ = bob.raw("GET", "/api/models", nil)
	if code != http.StatusOK || strings.Contains(body, "server-secret") {
		t.Fatalf("pick list = %d %s", code, body)
	}

	if err := json.Unmarshal([]byte(body), &picks); err != nil {
		t.Fatal(err)
	}

	byName := map[string]pick{}
	for _, p := range picks.Models {
		byName[p.Name] = p
	}

	if !byName["Remote"].Default || byName["Local"].Default || byName["Remote"].NeedsKey || byName["Remote"].KeyName != "REMOTE_KEY" {
		t.Fatalf("picks = %+v", picks.Models)
	}

	alice.expect(http.StatusNoContent, "DELETE", "/api/admin/models/"+remote.Model.ID, nil)
	bob.expect(http.StatusOK, "GET", "/api/models", nil, &picks)
	if len(picks.Models) != 1 || !picks.Models[0].Default {
		t.Fatalf("after deleting the default = %+v", picks.Models)
	}
}

// The mail relay is saved by an administrator, the password never read
// back and kept when the form leaves it out.
func TestAnAdministratorSetsTheMailRelay(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	type view struct {
		Mail struct {
			Host        string `json:"host"`
			Port        int    `json:"port"`
			Username    string `json:"username"`
			HasPassword bool   `json:"has_password"`
			Enabled     bool   `json:"enabled"`
		} `json:"mail"`
	}

	var got view

	bob.expect(http.StatusForbidden, "GET", "/api/admin/settings/mail", nil)
	alice.expect(http.StatusOK, "GET", "/api/admin/settings/mail", nil, &got)
	if got.Mail.Enabled || got.Mail.Port != 587 {
		t.Fatalf("a fresh relay = %+v", got.Mail)
	}

	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/mail", map[string]any{"host": "smtp.example.com", "port": 587, "from": "not an address"})
	code, body, _ := alice.raw("PUT", "/api/admin/settings/mail", map[string]any{"host": "smtp.example.com", "port": 587,
		"username": "relay", "password": "relay-pass", "from": "Gosint <gosint@example.com>", "starttls": true})
	if code != http.StatusOK || strings.Contains(body, "relay-pass") {
		t.Fatalf("save = %d %s", code, body)
	}

	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/mail", map[string]any{"host": "smtp.example.com", "port": 2525,
		"username": "relay", "from": "gosint@example.com", "starttls": true}, &got)
	if !got.Mail.Enabled || got.Mail.Port != 2525 || !got.Mail.HasPassword {
		t.Fatalf("a save leaving the password out = %+v", got.Mail)
	}
}

// The worker's settings are saved within bounds, a relative cache
// directory refused, and they apply on this node at once.
func TestAnAdministratorTunesTheWorker(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/worker", map[string]any{"concurrency": 0,
		"poll_seconds": 2, "lease_seconds": 120, "max_attempts": 3})
	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/worker", map[string]any{"enabled": true, "cache_dir": "cache",
		"concurrency": 2, "poll_seconds": 2, "lease_seconds": 120, "max_attempts": 3})

	dir := t.TempDir()
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/worker", map[string]any{"enabled": false, "cache_dir": dir,
		"concurrency": 9, "poll_seconds": 5, "lease_seconds": 300, "max_attempts": 4})

	if got := rt.WorkerTuning.Get(); got.Enabled || got.Concurrency != 9 || got.Poll != 5*time.Second || got.Lease != 5*time.Minute || got.MaxAttempts != 4 {
		t.Fatalf("tuning = %+v", got)
	}

	if rt.CacheDir.Get() != dir {
		t.Fatalf("cache dir = %s", rt.CacheDir.Get())
	}

	var back struct {
		Worker struct {
			Concurrency int `json:"concurrency"`
		} `json:"worker"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/settings/worker", nil, &back)
	if back.Worker.Concurrency != 9 {
		t.Fatalf("read back = %+v", back)
	}
}

// Each tool takes an absolute path and its own private address switch,
// an unknown tool or a relative path is refused, and the web switch
// reaches the runs at once.
func TestAnAdministratorSetsTheTools(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	type view struct {
		Tools []struct {
			Name         string `json:"name"`
			Path         string `json:"path"`
			AllowPrivate bool   `json:"allow_private"`
			Found        string `json:"found"`
		} `json:"tools"`
	}

	var got view

	alice.expect(http.StatusOK, "GET", "/api/admin/settings/tools", nil, &got)
	if len(got.Tools) < 2 || got.Tools[0].Name != "web" {
		t.Fatalf("tools = %+v", got.Tools)
	}

	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/tools", map[string]any{"tools": map[string]any{"nope": map[string]any{}}})
	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/tools", map[string]any{"tools": map[string]any{"nmap": map[string]any{"path": "bin/nmap"}}})
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/tools", map[string]any{"tools": map[string]any{
		"web": map[string]any{"allow_private": true}, "nmap": map[string]any{"path": "/nowhere/nmap"}}}, &got)

	if s := rt.ToolSettings.Get(); !s.Get("web").AllowPrivate || s.Get("nmap").Path != "/nowhere/nmap" || s.Get("nmap").AllowPrivate {
		t.Fatalf("settings = %+v", s)
	}

	for _, tl := range got.Tools {
		if tl.Name == "nmap" && tl.Found != "" {
			t.Errorf("a missing program is found at %s", tl.Found)
		}
	}
}

// The file store is saved only once it answers, the S3 secret never
// read back, and the limits apply at once. Changing the backend leaves
// the stored files where they are.
func TestAnAdministratorSetsTheFileStore(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var got struct {
		Files struct {
			Backend     string `json:"backend"`
			MaxSizeMB   int64  `json:"max_size_mb"`
			RetainHours int    `json:"retain_hours"`
			Dir         string `json:"dir"`
			S3          struct {
				Bucket       string `json:"bucket"`
				HasSecretKey bool   `json:"has_secret_key"`
			} `json:"s3"`
		} `json:"files"`
	}

	alice.expect(http.StatusOK, "GET", "/api/admin/settings/files", nil, &got)
	if got.Files.Backend != "fs" || got.Files.MaxSizeMB != 32 {
		t.Fatalf("a fresh store = %+v", got.Files)
	}

	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/files", map[string]any{"backend": "fs", "dir": "files",
		"max_size_mb": 10, "retain_hours": 24})
	alice.expect(http.StatusBadRequest, "PUT", "/api/admin/settings/files", map[string]any{"backend": "s3",
		"max_size_mb": 10, "retain_hours": 24, "s3": map[string]any{}})

	dir := t.TempDir()
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/files", map[string]any{"backend": "fs", "dir": dir,
		"max_size_mb": 10, "retain_hours": 48}, &got)

	if l := rt.FileLimits.Get(); l.MaxSize != 10<<20 || l.RetainDeleted != 48*time.Hour || got.Files.Dir != dir {
		t.Fatalf("limits = %+v, view = %+v", l, got.Files)
	}

	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/files", map[string]any{"backend": "db",
		"max_size_mb": 10, "retain_hours": 48}, &got)
	if got.Files.Backend != "db" {
		t.Fatalf("view = %+v", got.Files)
	}
}

// A fresh install tells the sign-in page to register the first
// account instead.
func TestAFreshInstallAsksForSetup(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	nobody := &api{t: t, srv: srv}

	var info struct {
		SetupRequired bool `json:"setup_required"`
	}

	nobody.expect(http.StatusOK, "GET", "/api/auth/info", nil, &info)
	if !info.SetupRequired {
		t.Fatal("a fresh install does not ask for setup")
	}

	signUp(t, srv, "alice@example.com")
	nobody.expect(http.StatusOK, "GET", "/api/auth/info", nil, &info)
	if info.SetupRequired {
		t.Fatal("an install with an account still asks for setup")
	}
}
