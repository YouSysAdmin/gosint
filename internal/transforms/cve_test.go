// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"net/http"
	"slices"
	"testing"
	"time"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/cve"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/epss"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nvd"
	"github.com/yousysadmin/gosint/internal/transforms/vulnerability"
)

// A CVE is filled from the NVD's record: the page, the English summary,
// the NVD's own newest CVSS, the CWE, the date, references and the KEV
// mark. An id the NVD does not know is reported, not drawn.
func TestCVEToDetails(t *testing.T) {
	h := newHarness(t)
	client := fakeNVD(t)
	first := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/epss": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("cve") != "CVE-2021-44228,CVE-2099-0001" {
				http.Error(w, "unexpected ids "+r.URL.Query().Get("cve"), http.StatusBadRequest)

				return
			}

			_, _ = w.Write([]byte(`{"data":[{"cve":"CVE-2021-44228","epss":"0.975","percentile":"0.99994"}]}`))
		},
	})
	log4j := h.node("cve", map[string]any{"id": "cve-2021-44228"})
	h.run(cve.CVEToDetails{Client: client, EPSS: epss.Client{BaseURL: first + "epss"}}, nil, log4j, h.node("cve", map[string]any{"id": "CVE-2099-0001"}))

	got, err := h.st.Graph.Node(t.Context(), h.sketch, log4j.ID)
	if err != nil {
		t.Fatal(err)
	}

	p := got.Properties
	if p["url"] != "https://nvd.nist.gov/vuln/detail/CVE-2021-44228" || p["cvss"] != 10.0 || p["severity"] != "critical" ||
		p["cvss_vector"] != "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H" || p["cwe"] != "CWE-502" ||
		p["known_exploited"] != true || p["kev_added"] != "2021-12-10T00:00:00Z" || p["epss"] != 0.975 || p["epss_percentile"] != 0.99994 ||
		p["published"] != "2021-12-10T10:15:09Z" ||
		p["description"] != "Apache Log4j2 JNDI features do not protect against attacker controlled LDAP." {
		t.Errorf("cve = %v", p)
	}

	if len(h.edges()) != 0 {
		t.Errorf("edges %q, want the record filled in place", h.edges())
	}

	if !slices.Contains(h.log, "warning: CVE-2099-0001: the NVD has no such record") {
		t.Errorf("an unknown id was not reported: %q", h.log)
	}
}

// A CVE becomes an open vulnerability with its shared fields, at the
// place given, referencing the CVE.
func TestCVEToVulnerability(t *testing.T) {
	h := newHarness(t)
	c := h.node("cve", map[string]any{"id": "CVE-2021-44228", "cvss": 10.0, "severity": "critical", "cwe": "CWE-502", "description": "Log4Shell."})
	h.run(cve.CVEToVulnerability{}, map[string]string{"affected": "https://app.example.com"}, c)
	h.expectEdges("CVE-2021-44228 @ app.example.com -REFERENCES-> CVE-2021-44228")

	nodes, _, _ := h.st.Graph.Graph(t.Context(), h.sketch)
	var vuln graphmodel.Node
	for _, n := range nodes {
		if n.Type == "vulnerability" {
			vuln = n
			p := n.Properties
			if p["status"] != "open" || p["cvss"] != 10.0 || p["cwe"] != "CWE-502" || p["cve"] != "CVE-2021-44228" || p["description"] != "Log4Shell." {
				t.Errorf("vulnerability = %v", p)
			}
		}
	}

	// Fixed and rescored by the investigator, a second run keeps both.
	if _, err := h.st.Graph.UpdateNode(t.Context(), h.sketch, vuln.ID, graphmodel.NodePatch{Properties: map[string]any{"status": "fixed", "cvss": 7.5}}); err != nil {
		t.Fatal(err)
	}

	h.run(cve.CVEToVulnerability{}, map[string]string{"affected": "https://app.example.com"}, c)
	again, _ := h.st.Graph.Node(t.Context(), h.sketch, vuln.ID)
	if again.Properties["status"] != "fixed" || again.Properties["cvss"] != 7.5 {
		t.Errorf("a second run reset the assessment: %v", again.Properties)
	}

	if nodes, _, _ := h.st.Graph.Graph(t.Context(), h.sketch); len(nodes) != 2 {
		t.Errorf("%d nodes after a second run, want 2", len(nodes))
	}
}

// A vulnerability takes the NVD's values only where it has none, the
// investigator's score stays, and the CVE is linked with its record.
// One without a CVE is reported.
func TestVulnerabilityToDetails(t *testing.T) {
	h := newHarness(t)
	v := h.node("vulnerability", map[string]any{"name": "Log4Shell in the login form", "cve": "cve-2021-44228",
		"affected": "https://app.example.com/login", "cvss": 8.1})
	h.run(vulnerability.VulnerabilityToDetails{Client: fakeNVD(t)}, nil, v,
		h.node("vulnerability", map[string]any{"name": "Weak TLS"}))

	got, _ := h.st.Graph.Node(t.Context(), h.sketch, v.ID)
	p := got.Properties
	if p["cvss"] != 8.1 || p["severity"] != "critical" || p["cwe"] != "CWE-502" || p["cve"] != "CVE-2021-44228" || p["description"] == nil {
		t.Errorf("vulnerability = %v", p)
	}

	h.expectEdges("Log4Shell in the login form @ app.example.com/login -REFERENCES-> CVE-2021-44228")
	if !slices.Contains(h.log, "warning: Weak TLS names no CVE, set its CVE field") {
		t.Errorf("no warning for a vulnerability without a CVE: %q", h.log)
	}
}

// fakeNVD answers Log4Shell's record and nothing else.
func fakeNVD(t *testing.T) nvd.Client {
	t.Helper()

	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/cves": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("cveId") != "CVE-2021-44228" {
				_, _ = w.Write([]byte(`{"vulnerabilities":[]}`))

				return
			}

			_, _ = w.Write([]byte(`{"vulnerabilities":[{"cve":{"id":"CVE-2021-44228","published":"2021-12-10T10:15:09.143",
				"descriptions":[{"lang":"es","value":"no"},{"lang":"en","value":"Apache Log4j2 JNDI features do not protect against attacker controlled LDAP."}],
				"metrics":{"cvssMetricV31":[{"type":"Secondary","cvssData":{"baseScore":9.0,"baseSeverity":"CRITICAL","vectorString":"CVSS:3.1/x"}},
					{"type":"Primary","cvssData":{"baseScore":10.0,"baseSeverity":"CRITICAL","vectorString":"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H"}}],
					"cvssMetricV2":[{"type":"Primary","cvssData":{"baseScore":9.3},"baseSeverity":"HIGH"}]},
				"weaknesses":[{"description":[{"value":"NVD-CWE-Other"},{"value":"CWE-502"}]}],
				"references":[{"url":"https://logging.apache.org/log4j/2.x/security.html"}],
				"cisaExploitAdd":"2021-12-10"}}]}`))
		},
	})

	none := time.Duration(0)

	return nvd.Client{BaseURL: base + "cves", Pace: &none}
}
