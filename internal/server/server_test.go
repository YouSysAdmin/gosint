// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/server"
)

func newServer(t *testing.T) *server.Server {
	t.Helper()

	console := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>shell</title>")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}

	rt := testRuntime(t, dbtest.SQLite(t), &env.Config{})

	srv, err := server.New(server.Options{Runtime: rt, Console: console})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return srv
}

func get(t *testing.T, srv *server.Server, path string) (int, string, http.Header) {
	t.Helper()

	resp, err := srv.App().Test(httptest.NewRequest(http.MethodGet, path, nil), testConfig)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	body, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(body), resp.Header
}

func TestHealthNamesTheBackend(t *testing.T) {
	code, body, _ := get(t, newServer(t), "/health")
	if code != http.StatusOK || !strings.Contains(body, `"database":"sqlite"`) {
		t.Errorf("GET /health = %d %s", code, body)
	}
}

func TestAnUnknownAPIPathAnswersTheEnvelope(t *testing.T) {
	code, body, _ := get(t, newServer(t), "/api/nope")
	if code != http.StatusNotFound || body != `{"error":"not found"}` {
		t.Errorf("GET /api/nope = %d %s", code, body)
	}
}

func TestADeepLinkGetsTheShell(t *testing.T) {
	code, body, _ := get(t, newServer(t), env.ConsolePath+"/investigations/42")
	if code != http.StatusOK || !strings.Contains(body, "shell") {
		t.Errorf("deep link = %d %s", code, body)
	}
}

// A chunk gone after an upgrade must be a 404, never the HTML shell.
func TestAMissingChunkIsNotTheHTMLShell(t *testing.T) {
	srv := newServer(t)

	code, body, hdr := get(t, srv, env.ConsolePath+"/assets/gone.js")
	if code != http.StatusNotFound || strings.Contains(body, "shell") {
		t.Errorf("missing chunk = %d %s", code, body)
	}

	if cc := hdr.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("missing chunk Cache-Control = %q, want no-store", cc)
	}

	code, _, hdr = get(t, srv, env.ConsolePath+"/assets/app.js")
	if code != http.StatusOK || !strings.Contains(hdr.Get("Cache-Control"), "immutable") {
		t.Errorf("present chunk = %d, Cache-Control %q", code, hdr.Get("Cache-Control"))
	}
}

// Embedded files have no modification time, so a date revalidation
// would answer 304 forever and keep a browser on an old console build.
func TestTheShellIsNeverRevalidatedByDate(t *testing.T) {
	srv := newServer(t)

	for _, path := range []string{env.ConsolePath + "/", env.ConsolePath + "/index.html", env.ConsolePath + "/login"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-Modified-Since", "Mon, 01 Jan 0001 00:00:00 GMT")

		resp, err := srv.App().Test(req, testConfig)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "shell") {
			t.Errorf("GET %s with If-Modified-Since = %d %q", path, resp.StatusCode, body)
		}

		if lm := resp.Header.Get("Last-Modified"); lm != "" {
			t.Errorf("GET %s Last-Modified = %q", path, lm)
		}
	}
}

// The shell carries the content policy, the API answers are never
// stored, and HSTS appears once the install is public over https.
func TestTheSecurityHeaders(t *testing.T) {
	console := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>shell</title>")}}
	serve := func(publicURL string) *server.Server {
		rt := testRuntime(t, dbtest.SQLite(t), &env.Config{Server: env.ServerConfig{PublicURL: publicURL}})
		srv, err := server.New(server.Options{Runtime: rt, Console: console})
		if err != nil {
			t.Fatalf("server.New: %v", err)
		}

		return srv
	}

	srv := serve("https://osint.example.com")

	_, _, shell := get(t, srv, env.ConsolePath+"/login")
	if csp := shell.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("shell CSP = %q", csp)
	}

	if hsts := shell.Get("Strict-Transport-Security"); hsts == "" {
		t.Error("no HSTS on an https install")
	}

	_, _, api := get(t, srv, "/api/nope")
	if cc := api.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("API Cache-Control = %q, want no-store", cc)
	}

	if csp := api.Get("Content-Security-Policy"); csp != "" {
		t.Errorf("an API answer carries the shell's policy %q", csp)
	}

	_, _, plain := get(t, serve("http://localhost:5001"), env.ConsolePath+"/login")
	if hsts := plain.Get("Strict-Transport-Security"); hsts != "" {
		t.Errorf("a plain http install sends HSTS %q", hsts)
	}
}
