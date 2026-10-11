// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/server"
)

// api is a test client holding one person's session cookie.
type api struct {
	t      *testing.T
	srv    *server.Server
	cookie string

	// cookieSet is how many responses carried the session cookie.
	cookieSet int
}

func newAPI(t *testing.T, db database.Database, tune ...func(*env.Config)) *server.Server {
	t.Helper()

	srv, _ := newAPIRuntime(t, db, tune...)

	return srv
}

// newAPIRuntime is newAPI also handing back the runtime, for a test that
// queues scans or publishes events behind the API's back.
func newAPIRuntime(t *testing.T, db database.Database, tune ...func(*env.Config)) (*server.Server, *env.Runtime) {
	t.Helper()

	cfg := &env.Config{Auth: env.AuthConfig{SessionTTL: time.Hour, OpenRegistration: true, PasskeysEnabled: true, RateLimitPerMinute: 1000}}
	for _, f := range tune {
		f(cfg)
	}

	rt := testRuntime(t, db, cfg)
	srv, err := server.New(server.Options{Runtime: rt})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}

	return srv, rt
}

// do sends a request and decodes a JSON answer into out when given.
func (a *api) do(method, path string, body any, out any, headers ...string) int {
	a.t.Helper()

	var r io.Reader
	if raw, ok := body.([]byte); ok {
		r = bytes.NewReader(raw)
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}

		r = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if a.cookie != "" {
		req.Header.Set("Cookie", auth.CookieName+"="+a.cookie)
	}

	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	resp, err := a.srv.App().Test(req, testConfig)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	for _, ck := range resp.Cookies() {
		if ck.Name == auth.CookieName {
			a.cookie = ck.Value
			a.cookieSet++
		}
	}

	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			a.t.Fatalf("%s %s: decode %q: %v", method, path, raw, err)
		}
	}

	return resp.StatusCode
}

// expect fails the test unless the request answers want.
func (a *api) expect(want int, method, path string, body any, out ...any) {
	a.t.Helper()

	var dst any
	if len(out) > 0 {
		dst = out[0]
	}

	if got := a.do(method, path, body, dst); got != want {
		a.t.Fatalf("%s %s = %d, want %d", method, path, got, want)
	}
}

func signUp(t *testing.T, srv *server.Server, email string) *api {
	t.Helper()

	a := &api{t: t, srv: srv}
	a.expect(http.StatusCreated, "POST", "/api/auth/register", map[string]string{"email": email, "password": "correct horse"})

	return a
}

// testConfig gives a request longer than Fiber's one second default,
// which bcrypt at cost 12 exceeds under the race detector.
var testConfig = fiber.TestConfig{Timeout: 30 * time.Second}

type idBody struct {
	ID string `json:"id"`
}

// A session ends at its expiry, is renewed once past half its life, and
// the cookie is re-sent only then.
func TestSessionExpiryAndRenewal(t *testing.T) {
	ttl := time.Hour
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t), func(c *env.Config) { c.Auth.SessionTTL = ttl })
	alice := signUp(t, srv, "alice@example.com")

	sum := sha256.Sum256([]byte(alice.cookie))
	sess, err := rt.Store.Session.Lookup(t.Context(), hex.EncodeToString(sum[:]), time.Now().UTC())
	if err != nil || sess == nil {
		t.Fatalf("session: %v %v", sess, err)
	}

	// Young: nothing moves, no cookie.
	sets := alice.cookieSet
	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)
	if alice.cookieSet != sets {
		t.Error("a fresh session was renewed")
	}

	// Past half its life: the expiry moves out to a full ttl and the
	// cookie follows.
	soon := time.Now().UTC().Add(ttl / 4).Truncate(time.Second)
	if err := rt.Store.Session.Extend(t.Context(), sess.ID, soon); err != nil {
		t.Fatal(err)
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)
	renewed, _ := rt.Store.Session.Lookup(t.Context(), hex.EncodeToString(sum[:]), time.Now().UTC())
	if renewed == nil || !renewed.ExpiresAt.After(soon.Add(ttl/2)) {
		t.Errorf("expiry after renewal = %v, want about %v later", renewed, ttl)
	}

	if alice.cookieSet != sets+1 {
		t.Errorf("cookie sent %d times across the renewal, want once", alice.cookieSet-sets)
	}

	// Expired: signed out.
	if err := rt.Store.Session.Extend(t.Context(), sess.ID, time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}

	alice.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
}

// Renewal stops at auth.session_max_life from the sign-in, and past it
// the session is gone however recently it was used.
func TestASessionEndsAtItsMaxLife(t *testing.T) {
	ttl, maxLife := time.Hour, 40*time.Minute
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t), func(c *env.Config) {
		c.Auth.SessionTTL = ttl
		c.Auth.SessionMaxLife = maxLife
	})
	alice := signUp(t, srv, "alice@example.com")

	sum := sha256.Sum256([]byte(alice.cookie))
	hash := hex.EncodeToString(sum[:])
	sess, err := rt.Store.Session.Lookup(t.Context(), hash, time.Now().UTC())
	if err != nil || sess == nil {
		t.Fatalf("session: %v %v", sess, err)
	}

	// Past half its ttl: renewed, but only up to the cap.
	if err := rt.Store.Session.Extend(t.Context(), sess.ID, time.Now().UTC().Add(15*time.Minute)); err != nil {
		t.Fatal(err)
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil)
	renewed, _ := rt.Store.Session.Lookup(t.Context(), hash, time.Now().UTC())
	if renewed == nil || renewed.ExpiresAt.After(sess.CreatedAt.Add(maxLife).Add(time.Second)) {
		t.Fatalf("renewed past the cap: %+v, cap %v", renewed, sess.CreatedAt.Add(maxLife))
	}

	// Past the cap: signed out, the expiry notwithstanding.
	rt.Config.Auth.SessionMaxLife = time.Nanosecond
	alice.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
}

func TestSignUpSignInSignOut(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		alice := signUp(t, srv, "Alice@Example.com")

		var me struct {
			User struct {
				Email string `json:"email"`
			} `json:"user"`
		}

		alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
		if me.User.Email != "alice@example.com" {
			t.Errorf("email = %q, want it lowercased", me.User.Email)
		}

		again := &api{t: t, srv: srv}
		again.expect(http.StatusConflict, "POST", "/api/auth/register",
			map[string]string{"email": "alice@example.com", "password": "another one"})
		again.expect(http.StatusUnauthorized, "POST", "/api/auth/login",
			map[string]string{"email": "alice@example.com", "password": "wrong password"})
		again.expect(http.StatusUnauthorized, "POST", "/api/auth/login",
			map[string]string{"email": "nobody@example.com", "password": "whatever"})
		again.expect(http.StatusOK, "POST", "/api/auth/login",
			map[string]string{"email": "ALICE@example.com", "password": "correct horse"})
		again.expect(http.StatusOK, "GET", "/api/auth/me", nil)

		token := again.cookie
		again.expect(http.StatusNoContent, "POST", "/api/auth/logout", nil)

		// The old cookie is dead server side, not just cleared in the browser.
		again.cookie = token
		again.expect(http.StatusUnauthorized, "GET", "/api/auth/me", nil)
	})
}

func TestClosedRegistrationStillAdmitsTheFirstAccount(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db, func(c *env.Config) { c.Auth.OpenRegistration = false })
		signUp(t, srv, "first@example.com")

		late := &api{t: t, srv: srv}
		late.expect(http.StatusForbidden, "POST", "/api/auth/register",
			map[string]string{"email": "second@example.com", "password": "correct horse"})
	})
}

func TestAValidationRefusalNamesTheField(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	a := &api{t: t, srv: srv}

	var body struct {
		Fields []struct {
			Field string `json:"field"`
		} `json:"fields"`
	}

	if code := a.do("POST", "/api/auth/register", map[string]string{"email": "not-an-email", "password": "x"}, &body); code != http.StatusBadRequest {
		t.Fatalf("register = %d, want 400", code)
	}

	got := map[string]bool{}
	for _, f := range body.Fields {
		got[f.Field] = true
	}

	if !got["email"] || !got["password"] {
		t.Errorf("fields = %+v, want email and password", body.Fields)
	}
}

func TestEverythingButSignInNeedsASession(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	anon := &api{t: t, srv: srv}

	for _, path := range []string{"/api/auth/me", "/api/investigations", "/api/sketches", "/api/users/search?q=a"} {
		anon.expect(http.StatusUnauthorized, "GET", path, nil)
	}
}

func TestACrossSitePostIsRefused(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	if code := alice.do("POST", "/api/investigations", map[string]string{"name": "x"}, nil,
		"Origin", "https://evil.example"); code != http.StatusForbidden {
		t.Errorf("foreign origin = %d, want 403", code)
	}

	if code := alice.do("POST", "/api/investigations", map[string]string{"name": "x"}, nil,
		"Sec-Fetch-Site", "cross-site"); code != http.StatusForbidden {
		t.Errorf("cross-site fetch = %d, want 403", code)
	}

	if code := alice.do("POST", "/api/investigations", map[string]string{"name": "x"}, nil,
		"Origin", "http://example.com"); code != http.StatusCreated {
		t.Errorf("same origin = %d, want 201", code)
	}
}

// Each role gets exactly what it should, and a stranger cannot tell the
// investigation exists.
func TestRolesOnAnInvestigation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		owner := signUp(t, srv, "owner@example.com")
		viewer := signUp(t, srv, "viewer@example.com")
		editor := signUp(t, srv, "editor@example.com")
		admin := signUp(t, srv, "admin@example.com")
		stranger := signUp(t, srv, "stranger@example.com")

		var created struct {
			Investigation idBody `json:"investigation"`
		}

		owner.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &created)
		inv := "/api/investigations/" + created.Investigation.ID

		for email, role := range map[string]string{
			"viewer@example.com": "viewer", "editor@example.com": "editor", "admin@example.com": "admin",
		} {
			owner.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": email, "role": role})
		}

		owner.expect(http.StatusConflict, "POST", inv+"/members", map[string]string{"email": "viewer@example.com", "role": "editor"})
		owner.expect(http.StatusBadRequest, "POST", inv+"/members", map[string]string{"email": "stranger@example.com", "role": "owner"})

		stranger.expect(http.StatusNotFound, "GET", inv, nil)
		stranger.expect(http.StatusNotFound, "PUT", inv, map[string]string{"name": "mine"})

		var list struct {
			Investigations []struct {
				Role string `json:"current_user_role"`
			} `json:"investigations"`
		}

		viewer.expect(http.StatusOK, "GET", "/api/investigations", nil, &list)
		if len(list.Investigations) != 1 || list.Investigations[0].Role != "viewer" {
			t.Errorf("viewer's list = %+v", list.Investigations)
		}

		viewer.expect(http.StatusOK, "GET", inv, nil)
		viewer.expect(http.StatusForbidden, "PUT", inv, map[string]string{"name": "renamed"})
		viewer.expect(http.StatusForbidden, "POST", inv+"/sketches", map[string]string{"title": "s"})

		editor.expect(http.StatusOK, "PUT", inv, map[string]string{"name": "renamed"})
		editor.expect(http.StatusCreated, "POST", inv+"/sketches", map[string]string{"title": "s"})
		editor.expect(http.StatusForbidden, "POST", inv+"/members", map[string]string{"email": "stranger@example.com", "role": "viewer"})
		editor.expect(http.StatusForbidden, "DELETE", inv, nil)

		admin.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "stranger@example.com", "role": "viewer"})
		admin.expect(http.StatusForbidden, "DELETE", inv, nil)

		var members struct {
			Members []struct {
				UserID string `json:"user_id"`
				Role   string `json:"role"`
			} `json:"members"`
		}

		admin.expect(http.StatusOK, "GET", inv+"/members", nil, &members)
		if len(members.Members) != 5 || members.Members[0].Role != "owner" {
			t.Fatalf("members = %+v", members.Members)
		}

		ownerID := members.Members[0].UserID
		admin.expect(http.StatusNotFound, "PUT", inv+"/members/"+ownerID, map[string]string{"role": "viewer"})
		admin.expect(http.StatusNotFound, "DELETE", inv+"/members/"+ownerID, nil)

		owner.expect(http.StatusNoContent, "DELETE", inv, nil)
		owner.expect(http.StatusNotFound, "GET", inv, nil)
	})
}

type nodeBody struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Color      string         `json:"color"`
}

type graphBody struct {
	Nodes []nodeBody `json:"nodes"`
	Edges []struct {
		ID     string `json:"id"`
		Source string `json:"source"`
		Target string `json:"target"`
		Label  string `json:"label"`
	} `json:"edges"`
}

func TestBuildingASketchGraph(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		alice := signUp(t, srv, "alice@example.com")
		bob := signUp(t, srv, "bob@example.com")

		var inv struct {
			Investigation idBody `json:"investigation"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)

		var sk struct {
			Sketch idBody `json:"sketch"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/sketches",
			map[string]string{"title": "Map"}, &sk)
		base := "/api/sketches/" + sk.Sketch.ID

		bob.expect(http.StatusNotFound, "GET", base+"/graph", nil)

		var a, b, c struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{
			"type": "domain", "label": "example.com", "properties": map[string]any{"domain": "example.com"}, "color": "#123456"}, &a)
		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{
			"type": "domain", "label": "example.com", "properties": map[string]any{"root": true}}, &b)
		if a.Node.ID != b.Node.ID || b.Node.Properties["domain"] != "example.com" || b.Node.Color != "#123456" {
			t.Fatalf("re-adding an entity did not merge into it: %+v", b.Node)
		}

		alice.expect(http.StatusBadRequest, "POST", base+"/nodes", map[string]any{"type": "Domain!", "label": "x"})
		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "93.184.216.34"}, &c)

		alice.expect(http.StatusCreated, "POST", base+"/edges", map[string]any{
			"source": a.Node.ID, "target": c.Node.ID, "label": "RESOLVES_TO"})
		alice.expect(http.StatusBadRequest, "POST", base+"/edges", map[string]any{
			"source": "banana", "target": c.Node.ID, "label": "X"})

		var g graphBody
		alice.expect(http.StatusOK, "GET", base+"/graph", nil, &g)
		if len(g.Nodes) != 2 || len(g.Edges) != 1 || g.Edges[0].Source != a.Node.ID {
			t.Fatalf("graph = %+v", g)
		}

		alice.expect(http.StatusOK, "GET", base+"/nodes/"+c.Node.ID+"/neighbors", nil, &g)
		if len(g.Nodes) != 2 {
			t.Errorf("neighbors = %+v", g.Nodes)
		}

		alice.expect(http.StatusNotFound, "GET", base+"/nodes/not-a-uuid/neighbors", nil)

		var renamed struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusOK, "PATCH", base+"/nodes/"+c.Node.ID, map[string]any{"label": "93.184.216.35"}, &renamed)
		if renamed.Node.Label != "93.184.216.35" || renamed.Node.Properties["address"] != "93.184.216.35" {
			t.Errorf("relabelled node = %+v, want the address to follow the label", renamed.Node)
		}

		alice.expect(http.StatusBadRequest, "PATCH", base+"/nodes/"+c.Node.ID, map[string]any{"label": "not an ip"})
		alice.expect(http.StatusBadRequest, "POST", base+"/nodes", map[string]any{"type": "nosuchtype", "label": "x"})
		alice.expect(http.StatusOK, "PUT", base+"/nodes/positions", map[string]any{
			"positions": []map[string]any{{"node_id": c.Node.ID, "x": 1.5, "y": 2}}})

		var merged struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusOK, "POST", base+"/nodes/merge", map[string]any{
			"node_ids": []string{a.Node.ID, c.Node.ID}, "keep_id": a.Node.ID}, &merged)
		if merged.Node.ID != a.Node.ID {
			t.Errorf("merge kept %s, want %s", merged.Node.ID, a.Node.ID)
		}

		var export struct {
			Sketch idBody     `json:"sketch"`
			Nodes  []nodeBody `json:"nodes"`
		}

		alice.expect(http.StatusOK, "POST", base+"/export", map[string]any{}, &export)
		if export.Sketch.ID != sk.Sketch.ID || len(export.Nodes) != 1 {
			t.Errorf("export = %+v", export)
		}

		var count struct {
			Count int `json:"count"`
		}

		alice.expect(http.StatusOK, "POST", base+"/nodes/delete", map[string]any{"ids": []string{a.Node.ID}}, &count)
		if count.Count != 1 {
			t.Errorf("deleted %d, want 1", count.Count)
		}

		alice.expect(http.StatusNoContent, "DELETE", base, nil)
		alice.expect(http.StatusNotFound, "GET", base, nil)
	})
}

// A wire node is flat: style fields sit beside type and label.
func TestANodeOnTheWireIsFlat(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var inv struct {
		Investigation idBody `json:"investigation"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)

	var sk struct {
		Sketch idBody `json:"sketch"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/sketches",
		map[string]string{"title": "Map"}, &sk)

	var raw map[string]map[string]any
	alice.expect(http.StatusCreated, "POST", "/api/sketches/"+sk.Sketch.ID+"/nodes",
		map[string]any{"type": "email", "label": "a@b.c", "icon": "mail"}, &raw)

	n := raw["node"]
	if n["icon"] != "mail" || n["Style"] != nil || !strings.Contains(n["id"].(string), "-") {
		t.Errorf("node on the wire = %v", n)
	}

	// An entity added by hand says who added it.
	meta, _ := n["metadata"].(map[string]any)
	if o, _ := meta["origin"].(map[string]any); o["via"] != "hand" || o["user"] == nil || o["at"] == nil {
		t.Errorf("origin of a hand-made node = %v", o)
	}
}

// An nmap report made elsewhere imports as hosts, ports and names.
func TestAnNmapReportImports(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	report := []byte(`<?xml version="1.0"?><nmaprun scanner="nmap">
<host><status state="up"/><address addr="203.0.113.9" addrtype="ipv4"/><address addr="00:11:22:33:44:55" addrtype="mac"/>
<hostnames><hostname name="mail.example.com." type="PTR"/></hostnames>
<ports><port protocol="tcp" portid="25"><state state="open"/><service name="smtp" product="Postfix smtpd"/></port>
<port protocol="tcp" portid="80"><state state="closed"/><service name="http"/></port>
<port protocol="udp" portid="161"><state state="open|filtered"/><service name="snmp"/></port></ports></host>
<host><status state="down"/><address addr="00:11:22:33:44:66" addrtype="mac"/></host>
</nmaprun>`)

	var result struct {
		NodesCreated int      `json:"nodes_created"`
		EdgesCreated int      `json:"edges_created"`
		Errors       []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", sketch+"/import/nmap", report, &result, "Content-Type", "application/xml")
	if result.NodesCreated != 4 || result.EdgesCreated != 3 || len(result.Errors) != 0 {
		t.Errorf("report imported as %+v", result)
	}

	var graph struct {
		Nodes []struct {
			Type       string         `json:"type"`
			Label      string         `json:"label"`
			Properties map[string]any `json:"properties"`
			Metadata   map[string]any `json:"metadata"`
		} `json:"nodes"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	labels := map[string]string{}
	for _, n := range graph.Nodes {
		labels[n.Type+":"+n.Label] = n.Label
		if o, _ := n.Metadata["origin"].(map[string]any); o["via"] != "import" || o["source"] != "nmap" || o["name"] != "alice@example.com" {
			t.Errorf("%s origin = %v, want the import by alice", n.Label, o)
		}
		if n.Label == "25 smtp (TCP)" && n.Properties["banner"] != "Postfix smtpd" {
			t.Errorf("port 25 = %v", n.Properties)
		}

		// A UDP port nobody answered on keeps nmap's honest state.
		if n.Label == "161 snmp (UDP)" && n.Properties["state"] != "open|filtered" {
			t.Errorf("port 161 = %v", n.Properties)
		}
	}

	for _, want := range []string{"ip:203.0.113.9", "domain:mail.example.com", "port:25 smtp (TCP)", "port:161 snmp (UDP)"} {
		if _, ok := labels[want]; !ok {
			t.Errorf("missing %s in %v", want, labels)
		}
	}

	// The same report again, with a second scan of a known host: the
	// ports attach to the existing ip, nothing is duplicated.
	again := []byte(`<nmaprun><host><address addr="203.0.113.9" addrtype="ipv4"/>
<ports><port protocol="tcp" portid="443"><state state="open"/><service name="https"/></port></ports></host></nmaprun>`)
	alice.expect(http.StatusOK, "POST", sketch+"/import/nmap", again, &result, "Content-Type", "application/xml")
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	ips := 0
	for _, n := range graph.Nodes {
		if n.Type == "ip" {
			ips++
		}
	}

	if ips != 1 || len(graph.Nodes) != 5 {
		t.Errorf("after a second report: %d ips, %d nodes, want 1 and 5", ips, len(graph.Nodes))
	}

	var refused struct {
		Error string `json:"error"`
	}

	alice.expect(http.StatusBadRequest, "POST", sketch+"/import/nmap", []byte(`{"nodes": []}`), &refused, "Content-Type", "application/xml")
	if !strings.Contains(refused.Error, "not an nmap XML report") {
		t.Errorf("a JSON body was answered %q", refused.Error)
	}
}

// A nuclei report becomes unverified findings under what they were
// found on, coloured by severity, with the CVEs they name, and the same
// report again changes nothing.
func TestImportNucleiReport(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)
	report := []byte(`{"template-id":"CVE-2021-44228","info":{"name":"Apache Log4j2 - RCE","severity":"critical","classification":{"cve-id":["cve-2021-44228"],"cwe-id":["cwe-502"],"cvss-score":10}},"type":"http","host":"https://app.example.com","matched-at":"https://app.example.com/login","ip":"203.0.113.7","timestamp":"2026-10-06T10:00:00Z"}
{"template-id":"openssh-detect","info":{"name":"OpenSSH Service - Detect","severity":"info"},"type":"network","host":"app.example.com:22","matched-at":"app.example.com:22","ip":"203.0.113.7","extracted-results":["SSH-2.0-OpenSSH_8.9p1"]}
garbage
`)

	var result struct {
		NodesCreated int      `json:"nodes_created"`
		EdgesCreated int      `json:"edges_created"`
		Errors       []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", sketch+"/import/nuclei", report, &result, "Content-Type", "application/x-ndjson")
	if result.NodesCreated != 6 || result.EdgesCreated != 5 || len(result.Errors) != 1 {
		t.Errorf("imported as %+v", result)
	}

	var graph struct {
		Nodes []struct {
			ID         string         `json:"id"`
			Type       string         `json:"type"`
			Label      string         `json:"label"`
			Flag       string         `json:"flag"`
			Properties map[string]any `json:"properties"`
		} `json:"nodes"`
		Edges []struct {
			Source string `json:"source"`
			Target string `json:"target"`
			Label  string `json:"label"`
		} `json:"edges"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	label := map[string]string{}
	for _, n := range graph.Nodes {
		label[n.ID] = n.Label
		if n.Type == "finding" && n.Properties["rule"] == "CVE-2021-44228" &&
			(n.Flag != "red" || n.Properties["status"] != "unverified" || n.Properties["severity"] != "critical" || n.Properties["source"] != "nuclei") {
			t.Errorf("log4j finding = %+v", n)
		}

		if n.Type == "cve" && (n.Label != "CVE-2021-44228" || n.Properties["cvss"] != 10.0 || n.Properties["cwe"] != "CWE-502") {
			t.Errorf("cve = %+v", n)
		}
	}

	var edges []string
	for _, e := range graph.Edges {
		edges = append(edges, label[e.Source]+" -"+e.Label+"-> "+label[e.Target])
	}

	for _, want := range []string{
		"app.example.com -RESOLVES_TO-> 203.0.113.7",
		"app.example.com -HAS_WEBSITE-> https://app.example.com/",
		"https://app.example.com/ -HAS_FINDING-> Apache Log4j2 - RCE @ app.example.com/login",
		"Apache Log4j2 - RCE @ app.example.com/login -REFERENCES-> CVE-2021-44228",
		"app.example.com -HAS_FINDING-> OpenSSH Service - Detect @ app.example.com:22",
	} {
		if !slices.Contains(edges, want) {
			t.Errorf("missing %q in %q", want, edges)
		}
	}

	// The investigator clears the flag after triage: the same report
	// again neither duplicates nor recolours.
	var log4j string
	for _, n := range graph.Nodes {
		if n.Type == "finding" && n.Properties["rule"] == "CVE-2021-44228" {
			log4j = n.ID
		}
	}

	alice.expect(http.StatusOK, "PATCH", sketch+"/nodes/"+log4j, map[string]any{"flag": ""})
	alice.expect(http.StatusOK, "POST", sketch+"/import/nuclei", report, &result, "Content-Type", "application/x-ndjson")
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	if len(graph.Nodes) != 6 {
		t.Errorf("the same report again made %d nodes, want 6", len(graph.Nodes))
	}

	for _, n := range graph.Nodes {
		if n.ID == log4j && n.Flag != "" {
			t.Errorf("a re-import put the flag back: %q", n.Flag)
		}
	}

	// Without a template id two names at one place are two findings, a
	// result naming no target is counted and left out.
	more := []byte(`{"template-id":"","info":{"name":"Header leak","severity":"low"},"type":"http","host":"https://app.example.com","matched-at":"https://app.example.com/"}
{"template-id":"","info":{"name":"Version leak","severity":"low"},"type":"http","host":"https://app.example.com","matched-at":"https://app.example.com/"}
{"template-id":"dns-only","info":{"name":"No target","severity":"info"},"type":"dns"}
`)
	alice.expect(http.StatusOK, "POST", sketch+"/import/nuclei", more, &result, "Content-Type", "application/x-ndjson")
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
	if len(graph.Nodes) != 8 || len(result.Errors) != 2 || !strings.Contains(result.Errors[0], "named no host") {
		t.Errorf("the second report: %d nodes, want 8, answer %+v", len(graph.Nodes), result)
	}

	// The report above was the first on that site without the log4j
	// hit, so it marked the finding missing then. A later report without
	// "Version leak" marks that one, dates the ones it holds, and the
	// next report holding log4j again clears its mark.
	later := []byte(`{"template-id":"openssh-detect","info":{"name":"OpenSSH Service - Detect","severity":"info"},"type":"network","host":"app.example.com:22","matched-at":"app.example.com:22","ip":"203.0.113.7"}
{"template-id":"","info":{"name":"Header leak","severity":"low"},"type":"http","host":"https://app.example.com","matched-at":"https://app.example.com/"}
`)
	alice.expect(http.StatusOK, "POST", sketch+"/import/nuclei", later, &result, "Content-Type", "application/x-ndjson")
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "marked missing") || !strings.Contains(result.Errors[0], "Version leak") {
		t.Errorf("the later report answered %+v", result)
	}

	state := func() (missing, seen string) {
		alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &graph)
		for _, n := range graph.Nodes {
			if n.ID == log4j {
				missing, _ = n.Properties["missing_since"].(string)
			}

			if n.Type == "finding" && n.Properties["rule"] == "openssh-detect" {
				seen, _ = n.Properties["last_seen"].(string)
			}
		}

		return
	}

	if missing, seen := state(); missing == "" || seen == "" {
		t.Errorf("after the later report: log4j missing_since %q, ssh last_seen %q", missing, seen)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/import/nuclei", report, &result, "Content-Type", "application/x-ndjson")
	if missing, _ := state(); missing != "" {
		t.Errorf("seen again, still missing since %q", missing)
	}
}

func TestExportImportRoundTrip(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	from, _ := sketchFor(t, alice)
	to, _ := sketchFor(t, alice)

	var a, b struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", from+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &a)
	alice.expect(http.StatusCreated, "POST", from+"/nodes", map[string]any{"type": "ip", "label": "93.184.216.34"}, &b)
	alice.expect(http.StatusCreated, "POST", from+"/edges", map[string]any{"source": a.Node.ID, "target": b.Node.ID, "label": "RESOLVES_TO"})

	var export map[string]any
	alice.expect(http.StatusOK, "POST", from+"/export", map[string]any{}, &export)

	var result struct {
		NodesCreated int      `json:"nodes_created"`
		EdgesCreated int      `json:"edges_created"`
		NodesSkipped int      `json:"nodes_skipped"`
		Errors       []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", to+"/import", map[string]any{"nodes": export["nodes"], "edges": export["edges"]}, &result)
	if result.NodesCreated != 2 || result.EdgesCreated != 1 {
		t.Errorf("our export imported as %+v", result)
	}

	// A refused entity is skipped with its relationships, the rest lands.
	mixed := map[string]any{
		"nodes": []map[string]any{
			{"id": "1", "type": "email", "label": "a@example.com", "properties": map[string]any{"email": "a@example.com"}},
			{"id": "2", "type": "domain", "label": "example.com", "properties": map[string]any{"domain": "example.com"}},
			{"id": "3", "type": "ip", "label": "not an ip"},
		},
		"edges": []map[string]any{
			{"source": "1", "target": "2", "label": "HAS_DOMAIN"},
			{"source": "3", "target": "2", "label": "X"},
		},
	}

	alice.expect(http.StatusOK, "POST", to+"/import", mixed, &result)
	if result.NodesCreated != 2 || result.NodesSkipped != 1 || result.EdgesCreated != 1 || len(result.Errors) != 1 {
		t.Errorf("a mixed import answered %+v", result)
	}
}

// A delete with_dependents takes what was found through the node, and
// clearing empties the sketch, which a viewer may do neither of.
func TestDeleteWithDependentsAndClear(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	var inv struct {
		Investigation idBody `json:"investigation"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)
	alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/members",
		map[string]string{"email": "bob@example.com", "role": "viewer"})
	carol := signUp(t, srv, "carol@example.com")
	alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/members",
		map[string]string{"email": "carol@example.com", "role": "editor"})

	var sk struct {
		Sketch idBody `json:"sketch"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/sketches",
		map[string]string{"title": "Map"}, &sk)
	base := "/api/sketches/" + sk.Sketch.ID

	// An editor may empty a sketch but not delete it, that is the owner's.
	carol.expect(http.StatusForbidden, "DELETE", base, nil)
	bob.expect(http.StatusForbidden, "DELETE", base, nil)
	carol.expect(http.StatusOK, "POST", base+"/clear", nil)

	add := func(typ, label string) string {
		var n struct {
			Node idBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": typ, "label": label}, &n)

		return n.Node.ID
	}

	domain := add("domain", "acme.io")
	org := add("organization", "Acme")
	person := add("individual", "Jane Roe")
	mail := add("email", "jane@acme.io")
	for _, e := range [][3]string{{domain, org, "RELATED_TO"}, {person, org, "WORKS_FOR"}, {person, mail, "HAS_EMAIL"}} {
		alice.expect(http.StatusCreated, "POST", base+"/edges", map[string]any{"source": e[0], "target": e[1], "label": e[2]})
	}

	var deps struct {
		IDs []string `json:"ids"`
	}

	bob.expect(http.StatusOK, "POST", base+"/nodes/dependents", map[string]any{"ids": []string{org}}, &deps)
	if len(deps.IDs) != 2 {
		t.Errorf("dependents of the org = %v, want the person and their email", deps.IDs)
	}

	bob.expect(http.StatusForbidden, "POST", base+"/clear", nil)

	var count struct {
		Count int `json:"count"`
	}

	alice.expect(http.StatusOK, "POST", base+"/nodes/delete", map[string]any{"ids": []string{org}, "with_dependents": true}, &count)
	if count.Count != 3 {
		t.Errorf("deleted %d, want the org, the person and the email", count.Count)
	}

	alice.expect(http.StatusOK, "POST", base+"/clear", nil, &count)
	if count.Count != 1 {
		t.Errorf("clear removed %d, want the domain left", count.Count)
	}

	var g struct {
		Nodes []idBody `json:"nodes"`
	}

	alice.expect(http.StatusOK, "GET", base+"/graph", nil, &g)
	if len(g.Nodes) != 0 {
		t.Errorf("after clear: %d nodes", len(g.Nodes))
	}
}

// A profile saves with an empty avatar, which clears it, and refuses an
// avatar that is not an http(s) URL.
func TestProfileAvatarMayBeEmptyButNotNonsense(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var me struct {
		User struct {
			FirstName string `json:"first_name"`
			AvatarURL string `json:"avatar_url"`
		} `json:"user"`
	}

	alice.expect(http.StatusOK, "PUT", "/api/auth/me", map[string]string{"first_name": "Ada", "avatar_url": ""}, &me)
	if me.User.FirstName != "Ada" || me.User.AvatarURL != "" {
		t.Errorf("profile = %+v", me.User)
	}

	alice.expect(http.StatusBadRequest, "PUT", "/api/auth/me", map[string]string{"avatar_url": "nonsense"})
	alice.expect(http.StatusOK, "PUT", "/api/auth/me", map[string]string{"avatar_url": "https://example.com/a.png"}, &me)
	if me.User.AvatarURL != "https://example.com/a.png" {
		t.Errorf("avatar = %q", me.User.AvatarURL)
	}
}

// A hand edit is undone and redone in order: a delete with dependents
// brings the entities back under their ids with their relationships,
// an edit its old label, a clear the whole sketch, and a new edit after
// an undo drops the redo.
func TestUndoAndRedoHandEdits(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	var a, b struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &a)
	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "ip", "label": "203.0.113.9"}, &b)
	alice.expect(http.StatusCreated, "POST", sketch+"/edges", map[string]any{"source": a.Node.ID, "target": b.Node.ID, "label": "RESOLVES_TO"})

	var next struct {
		Undo string `json:"undo"`
		Redo string `json:"redo"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/changes", nil, &next)
	if next.Undo != "link RESOLVES_TO" || next.Redo != "" {
		t.Errorf("after three edits: %+v", next)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/nodes/delete", map[string]any{"ids": []string{a.Node.ID}, "with_dependents": true})

	var g graphBody
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	if len(g.Nodes) != 0 {
		t.Fatalf("the delete left %d nodes", len(g.Nodes))
	}

	var done struct {
		Reverted string `json:"reverted"`
		Undo     string `json:"undo"`
		Redo     string `json:"redo"`
	}

	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil, &done)
	if done.Reverted != "delete 2 entities" || done.Redo != "delete 2 entities" || done.Undo != "link RESOLVES_TO" {
		t.Errorf("undo answered %+v", done)
	}

	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	ids := map[string]bool{}
	for _, n := range g.Nodes {
		ids[n.ID] = true
	}

	if !ids[a.Node.ID] || !ids[b.Node.ID] || len(g.Edges) != 1 || g.Edges[0].Label != "RESOLVES_TO" {
		t.Errorf("after the undo: %d nodes %v, edges %+v", len(g.Nodes), ids, g.Edges)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/redo", nil, &done)
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	if done.Reverted != "delete 2 entities" || len(g.Nodes) != 0 {
		t.Errorf("after the redo: %+v, %d nodes", done, len(g.Nodes))
	}

	// Back again, then a rename, undone to the old label. The rename
	// after an undo dropped the redo of the delete.
	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil, &done)
	alice.expect(http.StatusOK, "PATCH", sketch+"/nodes/"+a.Node.ID, map[string]any{"label": "renamed.com"})
	alice.expect(http.StatusOK, "GET", sketch+"/changes", nil, &next)
	if next.Undo != "edit example.com" || next.Redo != "" {
		t.Errorf("after the rename: %+v", next)
	}

	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil, &done)
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	labels := map[string]string{}
	for _, n := range g.Nodes {
		labels[n.ID] = n.Label
	}

	if labels[a.Node.ID] != "example.com" {
		t.Errorf("after undoing the rename: %v", labels)
	}

	// Clear and bring everything back.
	alice.expect(http.StatusOK, "POST", sketch+"/clear", nil)
	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil, &done)
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	if done.Reverted != "clear the sketch" || len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Errorf("after undoing the clear: %+v, %d nodes %d edges", done, len(g.Nodes), len(g.Edges))
	}

	// Another member undoes their own edits only.
	bob := signUp(t, srv, "bob@example.com")
	alice.expect(http.StatusCreated, "POST", investigationOf(t, alice, sketch)+"/members",
		map[string]string{"email": "bob@example.com", "role": "editor"})
	bob.expect(http.StatusBadRequest, "POST", sketch+"/undo", nil)
}

// A search finds entities across the sketches the caller may read, by
// a piece of the label or the entity itself, never in an investigation
// the caller is not a member of.
func TestSearchAcrossSketches(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")
	mine, mineID := sketchFor(t, alice)
	other, otherID := sketchFor(t, bob)

	var n struct {
		Node struct {
			ID  string `json:"id"`
			Key string `json:"key"`
		} `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", mine+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"}, &n)
	alice.expect(http.StatusCreated, "POST", mine+"/nodes", map[string]any{"type": "domain", "label": "example.com"})
	bob.expect(http.StatusCreated, "POST", other+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"})

	var found struct {
		Hits []struct {
			ID       string `json:"id"`
			SketchID string `json:"sketch_id"`
			Label    string `json:"label"`
		} `json:"hits"`
	}

	alice.expect(http.StatusOK, "GET", "/api/search?q=EXAMPLE", nil, &found)
	if len(found.Hits) != 2 {
		t.Errorf("alice finds %+v, want her two entities only", found.Hits)
	}

	for _, h := range found.Hits {
		if h.SketchID == otherID {
			t.Errorf("alice sees bob's sketch: %+v", h)
		}
	}

	// Bob joins alice's investigation and finds the same email in both
	// sketches, the panel's lookup leaving its own sketch out.
	alice.expect(http.StatusCreated, "POST", investigationOf(t, alice, mine)+"/members", map[string]string{"email": "bob@example.com", "role": "viewer"})
	bob.expect(http.StatusOK, "GET", "/api/search?type=email&key="+n.Node.Key, nil, &found)
	if len(found.Hits) != 2 {
		t.Errorf("bob finds %+v, want the email in both sketches", found.Hits)
	}

	bob.expect(http.StatusOK, "GET", "/api/search?type=email&key="+n.Node.Key+"&exclude="+otherID, nil, &found)
	if len(found.Hits) != 1 || found.Hits[0].SketchID != mineID {
		t.Errorf("excluding his own sketch, bob finds %+v", found.Hits)
	}

	bob.expect(http.StatusBadRequest, "GET", "/api/search", nil)
}

// A merge keeping one node runs the merged fields through its type like
// a hand edit: a value the type refuses is a 400, a new label renames
// the entity and its key follows.
func TestAMergeKeepingANodeGoesThroughItsType(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		alice := signUp(t, srv, "alice@example.com")

		var inv struct {
			Investigation idBody `json:"investigation"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)

		var sk struct {
			Sketch idBody `json:"sketch"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/sketches",
			map[string]string{"title": "Map"}, &sk)
		base := "/api/sketches/" + sk.Sketch.ID

		var a, b struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "1.1.1.1"}, &a)
		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "1.1.1.2"}, &b)

		alice.expect(http.StatusBadRequest, "POST", base+"/nodes/merge", map[string]any{
			"node_ids": []string{a.Node.ID, b.Node.ID}, "keep_id": a.Node.ID,
			"node": map[string]any{"properties": map[string]any{"latitude": "far north"}}})

		var merged struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusOK, "POST", base+"/nodes/merge", map[string]any{
			"node_ids": []string{a.Node.ID, b.Node.ID}, "keep_id": a.Node.ID,
			"node": map[string]any{"label": "1.1.1.3", "properties": map[string]any{"latitude": "50.5"}}}, &merged)
		if merged.Node.ID != a.Node.ID || merged.Node.Label != "1.1.1.3" ||
			merged.Node.Properties["address"] != "1.1.1.3" || merged.Node.Properties["latitude"] != 50.5 {
			t.Fatalf("merged = %+v", merged.Node)
		}

		// The key followed the label: adding the new address finds it.
		var again struct {
			Node nodeBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "1.1.1.3"}, &again)
		if again.Node.ID != a.Node.ID {
			t.Errorf("1.1.1.3 is a new node %s, want the merged %s", again.Node.ID, a.Node.ID)
		}
	})
}
