// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"

	"github.com/yousysadmin/gosint/internal/transforms/asn"
	"github.com/yousysadmin/gosint/internal/transforms/domain"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/crtsh"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nmap"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/subdomains"
	"github.com/yousysadmin/gosint/internal/transforms/ip"
	"github.com/yousysadmin/gosint/internal/transforms/org"
	"github.com/yousysadmin/gosint/internal/transforms/website"
)

func TestIPToPortsFromInternetDB(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/1.1.1.1": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"ip":"1.1.1.1","ports":[53,443],"hostnames":["one.one.one.one"]}`))
		},
	})

	h.run(ip.IPToPorts{Passive: ip.InternetDB{BaseURL: base}}, nil,
		h.node("ip", map[string]any{"address": "1.1.1.1"}),
		h.node("ip", map[string]any{"address": "192.0.2.1"}))

	h.expectEdges("1.1.1.1 -HAS_PORT-> 443 (TCP)", "1.1.1.1 -HAS_PORT-> 53 (TCP)")
}

// fakeNmap is a script standing in for the binary: it records its
// arguments and prints a report.
func fakeNmap(t *testing.T) (path, argsFile string) {
	t.Helper()

	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	path = filepath.Join(dir, "nmap")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\ncat <<'XML'\n" + `<?xml version="1.0"?>
<nmaprun scanner="nmap"><host><status state="up"/><address addr="203.0.113.9" addrtype="ipv4"/>
<ports><port protocol="tcp" portid="22"><state state="open"/><service name="ssh" product="OpenSSH" version="9.6"/></port>
<port protocol="tcp" portid="443"><state state="open"/><service name="https"/></port>
<port protocol="tcp" portid="8080"><state state="filtered"/><service name="http-proxy"/></port>
</ports></host><runstats><hosts up="1" down="0" total="1"/></runstats></nmaprun>` + "\nXML\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil { //nolint:gosec // an executable test stub
		t.Fatal(err)
	}

	return path, argsFile
}

func TestIPToPortsWithNmap(t *testing.T) {
	h := newHarness(t)
	path, argsFile := fakeNmap(t)
	active := ip.Nmap{Scanner: nmap.Scanner{Path: path}}

	h.run(ip.IPToPorts{Active: active}, map[string]string{"source": "active", "top_ports": "50"},
		h.node("ip", map[string]any{"address": "203.0.113.9"}))
	h.expectEdges("203.0.113.9 -HAS_PORT-> 22 ssh (TCP)", "203.0.113.9 -HAS_PORT-> 443 https (TCP)")

	args, _ := os.ReadFile(argsFile) //nolint:gosec // a file this test wrote
	for _, want := range []string{"-Pn", "--top-ports 50", "203.0.113.9"} {
		if !strings.Contains(string(args), want) {
			t.Errorf("nmap arguments %q lack %q", args, want)
		}
	}

	nodes, _, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range nodes {
		if n.Type == "port" && n.Label[:2] == "22" && n.Properties["banner"] != "OpenSSH 9.6" {
			t.Errorf("port 22 = %v", n.Properties)
		}
	}
}

// The server's own network is out of reach, as for the web client.
func TestNmapRefusesInternalAddresses(t *testing.T) {
	h := newHarness(t)
	path, argsFile := fakeNmap(t)

	h.run(ip.IPToPorts{Active: ip.Nmap{Scanner: nmap.Scanner{Path: path}}}, map[string]string{"source": "active", "top_ports": "50"},
		h.node("ip", map[string]any{"address": "10.0.0.1"}))

	if len(h.edges()) != 0 || !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "refusing") }) {
		t.Errorf("edges %q log %q", h.edges(), h.log)
	}

	if _, err := os.Stat(argsFile); err == nil {
		t.Error("nmap ran against an internal address")
	}
}

// worker.allow_private_targets lets a lab's addresses be scanned.
func TestNmapScansALabWhenAllowed(t *testing.T) {
	h := newHarness(t)
	h.allowPrivate = true
	path, argsFile := fakeNmap(t)

	h.run(ip.IPToPorts{Active: ip.Nmap{Scanner: nmap.Scanner{Path: path}}}, map[string]string{"source": "active", "top_ports": "50"},
		h.node("ip", map[string]any{"address": "10.10.12.34"}))

	if _, err := os.Stat(argsFile); err != nil {
		t.Errorf("nmap did not run against the lab address: %v, log %q", err, h.log)
	}
}

func TestNmapMissingIsReported(t *testing.T) {
	_, err := nmap.Scanner{Path: filepath.Join(t.TempDir(), "nmap")}.Scan(t.Context(), "203.0.113.9", 10)
	if !errors.Is(err, nmap.ErrNotInstalled) {
		t.Errorf("err = %v", err)
	}
}

func TestASNTransformsOverRIPEstat(t *testing.T) {
	h := newHarness(t)
	ok := func(body string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"ok","data":` + body + `}`))
		}
	}

	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/network-info/":       ok(`{"asns":["15169"],"prefix":"8.8.8.0/24"}`),
		"/as-overview/":        ok(`{"holder":"GOOGLE - Google LLC"}`),
		"/rir-stats-country/":  ok(`{"located_resources":[{"location":"US"}]}`),
		"/announced-prefixes/": ok(`{"prefixes":[{"prefix":"8.8.8.0/24"},{"prefix":"0.0.0.0/0"}]}`),
		"/searchcomplete/":     ok(`{"categories":[{"category":"ASNs","suggestions":[{"value":"AS15169","description":"GOOGLE - Google LLC"}]}]}`),
	})

	addr := h.node("ip", map[string]any{"address": "8.8.8.8"})
	h.run(ip.IPToASN{BaseURL: base}, nil, addr)

	as := h.node("asn", map[string]any{"asn_str": "AS15169"})
	h.run(asn.ASNToCIDRs{BaseURL: base}, nil, as)
	h.run(org.OrgToASN{BaseURL: base}, nil, h.node("organization", map[string]any{"name": "Google"}))

	h.expectEdges(
		"8.8.8.8 -BELONGS_TO-> AS15169 - GOOGLE - Google LLC",
		"AS15169 - GOOGLE - Google LLC -ANNOUNCES-> 8.8.8.0/24",
		"Google -BELONGS_TO-> AS15169 - GOOGLE - Google LLC",
	)
}

// subfinder results win, and crt.sh is not asked when they exist.
func TestSubdomainsPreferThePassiveSources(t *testing.T) {
	h := newHarness(t)
	asked := false
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) { asked = true; _, _ = w.Write([]byte(`[]`)) },
	})

	passive := func(context.Context, string) ([]string, error) {
		return []string{"api.example.com", "API.example.com", "notexample.com"}, nil
	}

	h.run(domain.DomainToSubdomains{BaseURL: base, Passive: passive}, nil, h.node("domain", map[string]any{"domain": "example.com"}))
	h.expectEdges("example.com -HAS_SUBDOMAIN-> api.example.com")

	if asked {
		t.Error("crt.sh was asked although subfinder found subdomains")
	}
}

// crt.sh answers 502 while overloaded, a later attempt usually works.
func TestCrtshIsAskedAgainAfterA5xx(t *testing.T) {
	fastCrtshRetries(t)
	h := newHarness(t)
	calls := 0
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls < 3 {
				http.Error(w, "busy", http.StatusBadGateway)

				return
			}

			_, _ = w.Write([]byte(`[{"name_value":"mx.example.com"}]`))
		},
	})

	h.run(domain.DomainToSubdomains{BaseURL: base}, nil, h.node("domain", map[string]any{"domain": "example.com"}))
	h.expectEdges("example.com -HAS_SUBDOMAIN-> mx.example.com")

	if calls != 3 {
		t.Errorf("crt.sh asked %d times, want 3", calls)
	}
}

func TestCrtshDownIsReportedAfterTheLastAttempt(t *testing.T) {
	fastCrtshRetries(t)
	h := newHarness(t)
	calls := 0
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			calls++
			http.Error(w, "down", http.StatusBadGateway)
		},
	})

	h.run(domain.DomainToSubdomains{BaseURL: base}, nil, h.node("domain", map[string]any{"domain": "example.com"}))

	if calls != 3 {
		t.Errorf("crt.sh asked %d times, want 3", calls)
	}

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "3 attempts, crt.sh is likely down") }) {
		t.Errorf("the outage was not reported: %q", h.log)
	}
}

// fastCrtshRetries makes the retry pauses negligible for the test.
func fastCrtshRetries(t *testing.T) {
	t.Helper()

	saved := crtsh.Retries
	crtsh.Retries = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { crtsh.Retries = saved })
}

// A website's subdomains come from the free sources by default, no key
// asked, and from c99.nl when chosen, with the address and the
// Cloudflare flag it adds. The c99 source without a key fails.
func TestWebsiteSubdomainsFreeByDefault(t *testing.T) {
	passive := func(context.Context, string) ([]string, error) {
		return []string{"api.acme.io", "www.acme.io", "notacme.io"}, nil
	}

	c99Asked := false
	c99 := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, r *http.Request) {
			c99Asked = true
			if r.URL.Query().Get("key") != "k" || r.URL.Query().Get("domain") != "acme.io" {
				http.Error(w, "bad query", http.StatusBadRequest)

				return
			}

			_, _ = w.Write([]byte(`{"success":true,"subdomains":[{"subdomain":"mail.acme.io","ip":"192.0.2.9","cloudflare":true}]}`))
		},
	})

	e := website.WebsiteToSubdomains{BaseURL: c99, Finder: subdomains.Finder{CrtURL: c99 + "/crt", Passive: passive}}

	free := newHarness(t)
	free.run(e, nil, free.node("website", map[string]any{"url": "https://www.acme.io"}))
	free.expectEdges("https://www.acme.io/ -IS_SUBDOMAIN-> https://api.acme.io/")
	if c99Asked {
		t.Error("c99.nl was asked on the free source")
	}

	paid := newHarness(t)
	paid.run(e, map[string]string{"source": "c99", "C99_API_KEY": "k"}, paid.node("website", map[string]any{"url": "https://acme.io"}))
	nodes, _, _ := paid.st.Graph.Graph(t.Context(), paid.sketch)
	var mail map[string]any
	for _, n := range nodes {
		if n.Label == "https://mail.acme.io/" {
			mail = n.Properties
		}
	}

	if mail == nil || mail["description"] != "IP address: 192.0.2.9" {
		t.Errorf("c99 subdomain = %v, edges %q", mail, paid.edges())
	}

	x := transform.NewExec(transform.ExecConfig{Params: map[string]string{"source": "c99"}})
	if err := e.Run(t.Context(), x, nil); err == nil || !strings.Contains(err.Error(), "C99_API_KEY") {
		t.Errorf("c99 without a key = %v, want the key named", err)
	}
}
