// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	chatmodel "github.com/yousysadmin/gosint/internal/models/chat"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// hexIndex is a keyed hash as an index column holds it.
var hexIndex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sqliteBytes is everything the sqlite file holds on disk, the WAL
// folded in first.
func sqliteBytes(t *testing.T, db database.Database) []byte {
	t.Helper()

	if _, err := db.DB().ExecContext(t.Context(), `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}

	var (
		seq        int
		name, path string
	)

	if err := db.DB().QueryRowContext(t.Context(), `PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}

	var all []byte
	for _, p := range []string{path, path + "-wal"} {
		b, err := os.ReadFile(p) //nolint:gosec // a test temp dir
		if err == nil {
			all = append(all, b...)
		}
	}

	return all
}

// Nothing a person types or a transform finds reaches the disk in
// clear: not the database, not the files.
func TestNoClearTextAtRest(t *testing.T) {
	db := dbtest.SQLite(t)
	srv, rt := newAPIRuntime(t, db)
	t.Cleanup(func() { _ = types.Builtin().SetCustom(nil) })

	const email = "nightjar.agent@example.com"
	alice := signUp(t, srv, email)
	alice.expect(http.StatusOK, "PUT", "/api/auth/me", map[string]string{"first_name": "Nightjar", "last_name": "Agentova"})
	if code := alice.do("POST", "/api/auth/login", map[string]string{"email": email, "password": "correct horse"}, nil,
		"User-Agent", "nightjar-browser/1.0"); code != http.StatusOK {
		t.Fatalf("login = %d", code)
	}

	alice.expect(http.StatusCreated, "POST", "/api/flows", map[string]any{"name": "nightjar recon", "description": "nightjar plan",
		"definition": map[string]any{"input": "domain", "steps": []map[string]any{{"id": "s1", "after": "input", "transform": "domain_to_ip"}}}})
	alice.expect(http.StatusCreated, "POST", "/api/transform-templates", map[string]any{"source": "name: nightjar-lookup\ncategory: Username\n" +
		"input:\n  type: username\n  key: value\nrequest:\n  url: \"https://nightjar.example/{{value}}\"\nresponse:\n  map:\n    username: login\n" +
		"output:\n  type: socialaccount\n"})
	alice.expect(http.StatusCreated, "POST", "/api/admin/types", map[string]any{"name": "nightjarbadge", "title": "Nightjar badge",
		"category": "person", "icon": "Tag", "color": "#336699",
		"fields": []map[string]any{{"name": "number", "title": "Number", "kind": "string", "primary": true}}})

	var inv struct {
		Investigation idBody `json:"investigation"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Operation Nightjar", "description": "nightjar brief"}, &inv)
	invPath := "/api/investigations/" + inv.Investigation.ID

	var sk struct {
		Sketch idBody `json:"sketch"`
	}

	alice.expect(http.StatusCreated, "POST", invPath+"/sketches", map[string]string{"title": "nightjar map"}, &sk)
	sketch := "/api/sketches/" + sk.Sketch.ID

	var node struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "nightjar.example"}, &node)
	var cred struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "credential",
		"properties": map[string]any{"username": "root", "service": "nightjar-box", "password": "hunter2-nightjar"}}, &cred)
	alice.expect(http.StatusCreated, "POST", sketch+"/edges", map[string]any{"source": cred.Node.ID, "target": node.Node.ID, "label": "NIGHTJAR_ON"})
	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "nightjarbadge", "label": "B-7"})

	fileBody := "nightjar file body"
	if code := alice.upload(sketch, node.Node.ID, "nightjar.txt", fileBody, nil); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph",
		"content": []any{map[string]any{"type": "text", "text": "nightjar evidence"}}}}}
	alice.expect(http.StatusCreated, "POST", invPath+"/analyses", map[string]any{"title": "nightjar findings", "content": doc})
	alice.expect(http.StatusCreated, "POST", "/api/secrets", map[string]string{"name": "WHOXY_API_KEY", "value": "nightjar-api-key"})
	alice.expect(http.StatusCreated, "POST", "/api/secrets", map[string]string{"name": "NIGHTJAR_TOKEN", "value": "x"})
	alice.expect(http.StatusCreated, "POST", "/api/admin/models", map[string]any{"name": "nightjar model",
		"base_url": "https://nightjar.example/v1", "model": "nightjar-7b", "api_key": "nightjar-server-key"})
	relay := map[string]any{"host": "smtp.nightjar.example", "port": 587, "username": "nightjar-relay",
		"from": "nightjar@example.com", "starttls": true}
	relay["password"] = "nightjar-relay-word"
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/mail", relay)
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/worker", map[string]any{"enabled": true,
		"cache_dir": filepath.Join(t.TempDir(), "nightjar-cache"), "concurrency": 2, "poll_seconds": 2, "lease_seconds": 120, "max_attempts": 3})
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/tools", map[string]any{"tools": map[string]any{
		"nmap": map[string]any{"path": "/opt/nightjar/nmap", "allow_private": true}}})
	alice.expect(http.StatusOK, "PUT", "/api/admin/settings/files", map[string]any{"backend": "fs",
		"dir": filepath.Join(t.TempDir(), "nightjar-files"), "max_size_mb": 8, "retain_hours": 24})

	ctx, now := t.Context(), time.Now().UTC()
	user, err := rt.Store.User.GetByEmail(ctx, email)
	if err != nil || user == nil || user.FirstName != "Nightjar" {
		t.Fatal(user, err)
	}

	chat := &chatmodel.Chat{ID: ids.New(), SketchID: sk.Sketch.ID, OwnerID: user.ID, Title: "nightjar chat", CreatedAt: now, UpdatedAt: now}
	if err := rt.Store.Chat.Create(ctx, chat); err != nil {
		t.Fatal(err)
	}

	if err := rt.Store.Chat.AddMessage(ctx, &chatmodel.Message{ID: ids.New(), ChatID: chat.ID, Role: "user",
		Content: "who runs nightjar", Context: []string{node.Node.ID}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	scan := &scanmodel.Scan{ID: ids.New(), SketchID: sk.Sketch.ID, OwnerID: user.ID, Kind: "transform", Name: "x",
		Status: scanmodel.StatusPending, Input: map[string]any{"q": "nightjar scan"}, CreatedAt: now}
	if err := rt.Store.Scan.Enqueue(ctx, scan); err != nil {
		t.Fatal(err)
	}

	if err := rt.Store.Log.Append(ctx, &logmodel.Entry{ID: ids.New(), SketchID: sk.Sketch.ID, Level: "info",
		Message: "found nightjar", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256([]byte(fileBody))
	digest := hex.EncodeToString(sum[:])
	raw := sqliteBytes(t, db)
	for _, clear := range []string{"ightjar", "IGHTJAR", "hunter2", "Agentova", "nightjar-browser", "domain_to_ip", digest} {
		if i := bytes.Index(raw, []byte(clear)); i >= 0 {
			t.Errorf("the database holds %q in clear: %q", clear, raw[max(0, i-120):min(len(raw), i+60)])
		}
	}

	// The columns matched by equality hold keyed hashes only.
	for _, q := range []string{`SELECT node_type FROM nodes`, `SELECT node_key FROM nodes`, `SELECT label FROM edges`,
		`SELECT email FROM users`, `SELECT name FROM flows`, `SELECT input_type FROM flows`, `SELECT name FROM vault_keys`,
		`SELECT name FROM transform_templates`, `SELECT name FROM custom_types`, `SELECT hash FROM attachments`} {
		rows, err := db.DB().QueryContext(ctx, q)
		if err != nil {
			t.Fatal(q, err)
		}

		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}

			if !hexIndex.MatchString(v) {
				t.Errorf("%s holds %q", q, v)
			}
		}

		_ = rows.Close()
	}

	// Every store reads it back.
	var got struct {
		Investigation struct {
			Name string `json:"name"`
		} `json:"investigation"`
	}

	alice.expect(http.StatusOK, "GET", invPath, nil, &got)
	if got.Investigation.Name != "Operation Nightjar" {
		t.Errorf("investigation = %+v", got)
	}

	var hits struct {
		Hits []struct {
			Label string `json:"label"`
		} `json:"hits"`
	}

	alice.expect(http.StatusOK, "GET", "/api/search?q=NIGHTJAR.EX", nil, &hits)
	if len(hits.Hits) != 1 || hits.Hits[0].Label != "nightjar.example" {
		t.Errorf("search = %+v", hits)
	}

	if secret, ok, err := vaultkey.Secret(ctx, rt, user.ID, "WHOXY_API_KEY"); err != nil || !ok || secret != "nightjar-api-key" { //nolint:gosec // a test value
		t.Errorf("secret = %q %v %v", secret, ok, err)
	}
}
