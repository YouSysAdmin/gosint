// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/cidr"
	"github.com/yousysadmin/gosint/internal/transforms/username"
	"github.com/yousysadmin/gosint/internal/transforms/website"
)

const samplePage = `<!doctype html><html><head><title>Shop</title>
<meta name="description" content="A shop">
<meta name="generator" content="WordPress 6.4">
<script>var hidden = "not text"</script></head>
<body><h1>Welcome</h1><p>Buy things</p></body></html>`

func sampleSite(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Server", "nginx/1.25.3")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(samplePage))
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

func TestWebsiteToTextLeavesScriptsOut(t *testing.T) {
	h := newHarness(t)

	// The website type wants a public host, so the node is made for one
	// and then pointed at the loopback server.
	n := h.node("website", map[string]any{"url": "https://example.com"})
	n.Properties["url"] = sampleSite(t)
	h.run(website.WebsiteToText{}, nil, n)

	if len(h.edges()) != 1 {
		t.Fatalf("edges = %q, log %q", h.edges(), h.log)
	}

	edge := h.edges()[0]
	if !strings.Contains(edge, "Welcome Buy things") || strings.Contains(edge, "not text") {
		t.Errorf("text edge = %q", edge)
	}
}

func TestTechDetectFindsTheStack(t *testing.T) {
	h := newHarness(t)
	n := h.node("website", map[string]any{"url": "https://example.com"})
	n.Properties["url"] = sampleSite(t)
	h.run(website.TechDetect{}, nil, n)

	edges := h.edges()
	for _, want := range []string{"Nginx", "WordPress"} {
		if !slices.ContainsFunc(edges, func(e string) bool { return strings.Contains(e, "-USES_TECHNOLOGY-> "+want) }) {
			t.Errorf("%s not detected: %q", want, edges)
		}
	}
}

func TestCIDRToIPsStopsAtTheLimit(t *testing.T) {
	h := newHarness(t)
	h.run(cidr.CIDRToIPs{}, map[string]string{"limit": "3"}, h.node("cidr", map[string]any{"network": "192.0.2.0/30"}))
	h.expectEdges("192.0.2.0/30 -CONTAINS-> 192.0.2.0", "192.0.2.0/30 -CONTAINS-> 192.0.2.1", "192.0.2.0/30 -CONTAINS-> 192.0.2.2")

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "stopped at 3") }) {
		t.Errorf("the cut was not reported: %q", h.log)
	}

	whole := newHarness(t)
	whole.run(cidr.CIDRToIPs{}, map[string]string{"limit": "10"}, whole.node("cidr", map[string]any{"network": "192.0.2.4/31"}))
	if len(whole.edges()) != 2 || slices.ContainsFunc(whole.log, func(l string) bool { return strings.Contains(l, "stopped") }) {
		t.Errorf("a /31 under the limit: edges %q log %q", whole.edges(), whole.log)
	}
}

// A website node must not become a way to read what only the server
// can reach.
func TestTheTargetClientRefusesInternalAddresses(t *testing.T) {
	c := transform.TargetHTTPClient("test", transform.Never)
	site := sampleSite(t)

	// The check sits at connect time, so a NAME resolving inward is
	// refused too, not only an address literal.
	byName := strings.Replace(site, "127.0.0.1", "localhost", 1)
	for _, url := range []string{site, byName, "http://10.0.0.1/", "http://169.254.169.254/latest/meta-data/", "http://[::1]/",
		"http://100.64.0.1/", "http://[64:ff9b::7f00:1]/"} {
		resp, err := c.Get(url) //nolint:noctx // test
		if err == nil {
			_ = resp.Body.Close()
		}

		if !errors.Is(err, transform.ErrPrivateTarget) {
			t.Errorf("GET %s = %v, want ErrPrivateTarget", url, err)
		}
	}
}

// worker.allow_private_targets opens the same client onto the lab: a
// site on the loopback answers.
func TestTheTargetClientReachesALabWhenAllowed(t *testing.T) {
	resp, err := transform.TargetHTTPClient("test", transform.Always).Get(sampleSite(t)) //nolint:noctx // test
	if err != nil {
		t.Fatalf("a loopback site with private targets allowed: %v", err)
	}

	_ = resp.Body.Close()
}

// A username is probed on every site, a 200 is a profile, a 404, the
// site's own "no such user" sentence or its own absent status is none,
// a bot wall or anything else is reported rather than drawn.
func TestUsernameToAccounts(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		// Routes match by prefix in no fixed order, so one handler per site.
		"/code/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/code/jane" {
				http.NotFound(w, r)

				return
			}

			_, _ = w.Write([]byte(`<h1>jane</h1>`))
		},
		"/forum":       func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("user: No such user.")) },
		"/busy/":       func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
		"/photos/jane": func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`ok`)) },
		"/wall/": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`<html><title>Client Challenge</title>`))
		},
		"/api/users": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("u") == "jane" {
				_, _ = w.Write([]byte(`{"name":"jane"}`))

				return
			}

			w.WriteHeader(http.StatusBadRequest)
		},
	})

	sites := []username.Site{
		{Name: "Code", URL: base + "code/{}"},
		{Name: "Forum", URL: base + "forum?id={}", Absent: "No such user."},
		{Name: "Busy", URL: base + "busy/{}"},
		{Name: "Photos", URL: base + "photos/{}"},
		{Name: "Wall", URL: base + "wall/{}"},
		{Name: "Api", URL: base + "api/users?u={}", AbsentStatus: http.StatusBadRequest},
	}

	jane := h.node("username", map[string]any{"value": "@jane"})
	h.run(username.UsernameToAccounts{Sites: sites}, nil, jane)
	h.expectEdges("jane -HAS_SOCIAL_ACCOUNT-> jane@Code", "jane -HAS_SOCIAL_ACCOUNT-> jane@Photos",
		"jane -HAS_SOCIAL_ACCOUNT-> jane@Api")

	for _, want := range [][2]string{{"Busy", "429"}, {"Wall", "bot check"}} {
		if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, want[0]) && strings.Contains(l, want[1]) }) {
			t.Errorf("%s was not reported as %q: %q", want[0], want[1], h.log)
		}
	}

	// Nobody is absent by status on the API, and gets no profile.
	nobody := newHarness(t)
	nobody.run(username.UsernameToAccounts{Sites: sites}, map[string]string{"sites": "api"},
		nobody.node("username", map[string]any{"value": "nobody"}))
	nobody.expectEdges()

	// The sites parameter narrows the probe.
	only := newHarness(t)
	only.run(username.UsernameToAccounts{Sites: sites}, map[string]string{"sites": "photos"},
		only.node("username", map[string]any{"value": "jane"}))
	only.expectEdges("jane -HAS_SOCIAL_ACCOUNT-> jane@Photos")
}
