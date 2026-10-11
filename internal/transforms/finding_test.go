// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"testing"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/finding"
)

// Confirming a finding draws its vulnerability with the fields carried
// over and the CVE its rule names, marks the finding confirmed, and a
// second run keeps the assessment made since.
func TestFindingToVulnerability(t *testing.T) {
	h := newHarness(t)
	f := h.node("finding", map[string]any{"title": "Apache Log4j2 - RCE", "rule": "CVE-2021-44228", "severity": "critical",
		"location": "https://app.example.com/login", "description": "Log4Shell.", "references": []any{"https://example.com/advisory"}})
	h.run(finding.FindingToVulnerability{}, nil, f)
	h.expectEdges("Apache Log4j2 - RCE @ app.example.com/login -CONFIRMED_AS-> Apache Log4j2 - RCE @ app.example.com/login")

	nodes, _, _ := h.st.Graph.Graph(t.Context(), h.sketch)
	var vuln graphmodel.Node
	for _, n := range nodes {
		switch n.Type {
		case "vulnerability":
			vuln = n
			p := n.Properties
			if p["status"] != "open" || p["severity"] != "critical" || p["cve"] != "CVE-2021-44228" || p["affected"] != "https://app.example.com/login" || p["discovered_at"] == nil {
				t.Errorf("vulnerability = %v", p)
			}
		case "finding":
			if n.Properties["status"] != "confirmed" {
				t.Errorf("finding status = %v", n.Properties["status"])
			}
		}
	}

	if _, err := h.st.Graph.UpdateNode(t.Context(), h.sketch, vuln.ID, graphmodel.NodePatch{Properties: map[string]any{"status": "mitigated", "cvss": 9.1}}); err != nil {
		t.Fatal(err)
	}

	h.run(finding.FindingToVulnerability{}, nil, f)
	again, _ := h.st.Graph.Node(t.Context(), h.sketch, vuln.ID)
	if again.Properties["status"] != "mitigated" || again.Properties["cvss"] != 9.1 {
		t.Errorf("a second run reset the assessment: %v", again.Properties)
	}

	if nodes, _, _ := h.st.Graph.Graph(t.Context(), h.sketch); len(nodes) != 2 {
		t.Errorf("%d nodes after a second run, want 2", len(nodes))
	}
}
