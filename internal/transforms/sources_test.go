// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/domain"
	"github.com/yousysadmin/gosint/internal/transforms/ip"
)

// The reputation sources lay what they know over the address and flag
// a bad one red, which a later clean verdict clears.
func TestReputationSourcesEnrichAndFlagAnAddress(t *testing.T) {
	h := newHarness(t)
	base := strings.TrimSuffix(api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/api/v2/check": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Key") != "k" || r.URL.Query().Get("maxAgeInDays") != "90" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"abuseConfidenceScore":92,"totalReports":14,"countryCode":"NL","isp":"Bad ISP","usageType":"Data Center","isTor":false,"lastReportedAt":"2026-10-01T00:00:00+00:00"}}`))
		},
		"/v3/community/": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("key") != "g" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"noise":true,"riot":false,"classification":"benign","name":"Shodan","last_seen":"2026-10-02"}`))
		},
		"/api/v3/ip_addresses/": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-apikey") != "v" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"attributes":{"last_analysis_stats":{"malicious":3,"suspicious":1,"harmless":60,"undetected":10},"reputation":-5,"as_owner":"Example AS","country":"NL"}}}`))
		},
		"/api/v2/hosts/": func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Basic ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"result":{"services":[{"port":22,"service_name":"SSH","transport_protocol":"TCP"},{"port":443,"service_name":"HTTP","transport_protocol":"TCP"}],"location":{"country":"Netherlands","city":"Amsterdam"},"autonomous_system":{"asn":64496,"name":"Example AS"}}}`))
		},
		"/shodan/host/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") != "sh" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"country_name":"Germany","city":"Berlin","latitude":52.5,"longitude":13.4,"asn":"AS64496","os":"Linux","tags":["vpn"],"hostnames":["mail.example.com"],"vulns":["CVE-2023-38408"],"last_update":"2026-10-03T00:00:00","data":[{"port":22,"transport":"tcp","product":"OpenSSH","version":"8.9","data":"SSH-2.0-OpenSSH_8.9","_shodan":{"module":"ssh"}},{"port":8080,"transport":"tcp","_shodan":{"module":"http"}}]}`))
		},
	}), "/")

	addr := h.node("ip", map[string]any{"address": "203.0.113.9"})
	h.run(ip.IPToAbuseIPDB{BaseURL: base}, map[string]string{"ABUSEIPDB_API_KEY": "k", "max_age_days": "90"}, addr)
	got, _ := h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if got.Flag != "red" || got.Properties["abuse_score"] != 92.0 || got.Properties["country"] != "NL" {
		t.Errorf("after AbuseIPDB: flag %q, props %v", got.Flag, got.Properties)
	}

	h.run(ip.IPToGreyNoise{BaseURL: base}, map[string]string{"GREYNOISE_API_KEY": "g"}, *got)
	got, _ = h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if got.Flag != "" || got.Properties["greynoise_classification"] != "benign" || got.Properties["greynoise_noise"] != true {
		t.Errorf("after GreyNoise: flag %q, props %v", got.Flag, got.Properties)
	}

	h.run(ip.IPToVirusTotal{BaseURL: base}, map[string]string{"VIRUSTOTAL_API_KEY": "v"}, *got)
	got, _ = h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if got.Flag != "red" || got.Properties["vt_malicious"] != 3.0 || got.Properties["isp"] != "Bad ISP" {
		t.Errorf("after VirusTotal: flag %q, props %v", got.Flag, got.Properties)
	}

	h.run(ip.IPToCensys{BaseURL: base}, map[string]string{"CENSYS_API_ID": "id", "CENSYS_API_SECRET": "s"}, *got)
	got, _ = h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if got.Properties["city"] != "Amsterdam" || got.Properties["asn"] != "AS64496" {
		t.Errorf("after Censys: props %v", got.Properties)
	}

	h.run(ip.IPToShodan{BaseURL: base}, map[string]string{"SHODAN_API_KEY": "sh"}, *got)
	got, _ = h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if got.Flag != "red" || got.Properties["city"] != "Amsterdam" || got.Properties["shodan_os"] != "Linux" || got.Properties["latitude"] != 52.5 {
		t.Errorf("after Shodan: flag %q, props %v", got.Flag, got.Properties)
	}

	h.expectEdges("203.0.113.9 -HAS_PORT-> 22 OpenSSH 8.9 (TCP)", "203.0.113.9 -HAS_PORT-> 443 http (TCP)",
		"203.0.113.9 -HAS_PORT-> 8080 http (TCP)", "203.0.113.9 -REVERSE_RESOLVES_TO-> mail.example.com",
		"203.0.113.9 -REFERENCES-> CVE-2023-38408")
}

// A key Shodan refuses fails the run instead of warning per address.
func TestShodanRefusedKeyFailsTheRun(t *testing.T) {
	h := newHarness(t)
	base := strings.TrimSuffix(api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/shodan/host/": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		},
	}), "/")

	addr := h.node("ip", map[string]any{"address": "203.0.113.9"})
	x := transform.NewExec(transform.ExecConfig{
		SketchID: h.sketch, Graph: h.st.Graph, Types: types.Builtin(), Log: &h.log,
		Params: map[string]string{"SHODAN_API_KEY": "bad"}, HTTP: transform.DefaultHTTPClient("gosint-test"),
	})
	if err := (ip.IPToShodan{BaseURL: base}).Run(t.Context(), x, []graphmodel.Node{addr}); err == nil {
		t.Fatal("a refused key did not fail the run")
	}
}

// The domain sources draw what they know around the domain: scanned
// pages with their addresses, verdicts with resolutions, subdomains
// with addresses, and addresses with the people they name.
func TestDomainIntelligenceSources(t *testing.T) {
	h := newHarness(t)
	base := strings.TrimSuffix(api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/api/v1/search/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") != "domain:example.com" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"results":[{"page":{"url":"https://example.com/login","domain":"example.com","ip":"203.0.113.9","title":"Login"},"task":{"time":"2026-10-01T10:00:00Z"}},{"page":{"url":"https://example.com/login","ip":"203.0.113.9"}}]}`))
		},
		"/api/v3/domains/example.com/resolutions": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"data":[{"attributes":{"ip_address":"203.0.113.10","date":1700000000}}]}`))
		},
		"/api/v3/domains/": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("x-apikey") != "v" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"attributes":{"last_analysis_stats":{"malicious":0,"suspicious":0,"harmless":70,"undetected":5},"reputation":12,"categories":{"a":"technology","b":"technology"}}}}`))
		},
		"/v1/domain/example.com/subdomains": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("APIKEY") != "s" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"subdomains":["www","mail","dev"]}`))
		},
		"/v1/domain/example.com": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"current_dns":{"a":{"values":[{"ip":"203.0.113.11"}]}}}`))
		},
		"/v2/domain-search": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("api_key") != "h" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"organization":"Example","emails":[{"value":"jane@example.com","type":"personal","confidence":95,"first_name":"Jane","last_name":"Roe","position":"CTO"},{"value":"info@example.com","type":"generic","confidence":80}]}}`))
		},
	}), "/")

	d := h.node("domain", map[string]any{"domain": "example.com"})
	h.run(domain.DomainToURLScan{BaseURL: base}, map[string]string{"max_results": "50"}, d)
	h.run(domain.DomainToVirusTotal{BaseURL: base}, map[string]string{"VIRUSTOTAL_API_KEY": "v"}, d)
	h.run(domain.DomainToSecurityTrails{BaseURL: base}, map[string]string{"SECURITYTRAILS_API_KEY": "s", "max_results": "2"}, d)
	h.run(domain.DomainToHunter{BaseURL: base}, map[string]string{"HUNTER_API_KEY": "h", "max_results": "100"}, d)

	h.expectEdges(
		"example.com -HAS_WEBSITE-> https://example.com/login",
		"https://example.com/login -RESOLVES_TO-> 203.0.113.9",
		"example.com -RESOLVED_TO-> 203.0.113.10",
		"example.com -HAS_SUBDOMAIN-> www.example.com",
		"example.com -HAS_SUBDOMAIN-> mail.example.com",
		"example.com -RESOLVES_TO-> 203.0.113.11",
		"example.com -HAS_EMAIL-> jane@example.com",
		"example.com -HAS_EMAIL-> info@example.com",
		"Jane Roe -HAS_EMAIL-> jane@example.com",
	)

	got, _ := h.st.Graph.Node(t.Context(), h.sketch, d.ID)
	if got.Flag != "" || got.Properties["vt_harmless"] != 70.0 {
		t.Errorf("domain after VirusTotal: flag %q, props %v", got.Flag, got.Properties)
	}

	if cats, _ := got.Properties["vt_categories"].([]any); len(cats) != 1 {
		t.Errorf("categories = %v, want one deduplicated", got.Properties["vt_categories"])
	}
}
