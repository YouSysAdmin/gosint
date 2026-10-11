// Gosint, Copyright (c) 2026 YouSysAdmin

// Package finding holds the transforms that start from a finding.
package finding

import (
	"context"
	"maps"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// carried are the finding's fields a vulnerability starts from.
var carried = map[string]string{"severity": "severity", "location": "affected", "description": "description", "references": "references"}

// FindingToVulnerability confirms a finding: a vulnerability is drawn
// from it (title, severity, place, description, references, the CVE
// when the rule is one), linked CONFIRMED_AS, and the finding's status
// becomes confirmed. Run again, the vulnerability already there keeps
// its assessment and only its empty fields are filled.
type FindingToVulnerability struct{}

// Meta describes the transform.
func (FindingToVulnerability) Meta() transform.Meta {
	return transform.Meta{Name: "finding_to_vulnerability", Category: "Security", Input: "finding", Output: "vulnerability",
		Description: "Confirm a finding: draw the vulnerability it is, with its severity, place, description and CVE, and mark the finding confirmed."}
}

// Run confirms each finding.
func (FindingToVulnerability) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, in := range inputs {
		v := existing(ctx, x, in)
		fresh := v == nil
		if fresh {
			v = map[string]any{"name": props.Str(in.Properties, "title"), "status": "open", "discovered_at": now}
		}

		for from, to := range carried {
			if !types.Truthy(v[to]) && types.Truthy(in.Properties[from]) {
				v[to] = in.Properties[from]
			}
		}

		if id := types.CVEID(props.Str(in.Properties, "rule")); id != "" && !types.Truthy(v["cve"]) {
			v["cve"] = id
		}

		n, err := x.NodeAndLink(ctx, in, "CONFIRMED_AS", "vulnerability", v)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if props.Str(in.Properties, "status") != "confirmed" {
			p := maps.Clone(in.Properties)
			p["status"] = "confirmed"
			if err := x.Annotate(ctx, in, p, ""); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}

		if fresh {
			x.Found("%s confirmed as %s", in.Label, n.Label)
		} else {
			x.Info("%s: already confirmed as %s", in.Label, n.Label)
		}
	}

	return nil
}

// existing is the vulnerability the finding was confirmed as before,
// nil when there is none.
func existing(ctx context.Context, x *transform.Exec, in graphmodel.Node) map[string]any {
	nodes, edges, err := x.Neighbours(ctx, in)
	if err != nil {
		return nil
	}

	byID := map[string]graphmodel.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	for _, e := range edges {
		if n, ok := byID[e.TargetID]; e.SourceID == in.ID && e.Label == "CONFIRMED_AS" && ok && n.Type == "vulnerability" {
			return maps.Clone(n.Properties)
		}
	}

	return nil
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{FindingToVulnerability{}}
}
