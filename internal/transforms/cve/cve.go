// Gosint, Copyright (c) 2026 YouSysAdmin

// Package cve holds the transforms that start from a CVE.
package cve

import (
	"context"
	"maps"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/epss"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nvd"
)

// CVEToDetails fills a CVE from the NVD: its page, description, CVSS
// score and vector, severity, weakness class, publication date,
// references and whether and when CISA listed it as exploited, and
// from FIRST its EPSS score and percentile.
type CVEToDetails struct {
	nvd.Client
	EPSS epss.Client
}

// Meta describes the transform.
func (CVEToDetails) Meta() transform.Meta {
	return transform.Meta{Name: "cve_to_details", Category: "Security", Input: "cve", Output: "cve",
		Description: "Fill a CVE from the NVD (page, description, CVSS, severity, CWE, published, references, CISA KEV) and FIRST (EPSS score and percentile).",
		Params:      []transform.Param{nvd.KeyParam}}
}

// Run asks the NVD for each CVE in turn.
func (e CVEToDetails) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	ids := make([]string, len(inputs))
	for i, in := range inputs {
		ids[i] = props.Str(in.Properties, "id")
	}

	// One EPSS request rates them all, the NVD one at a time.
	scores, err := e.EPSS.Lookup(ctx, x, ids)
	if err != nil {
		x.Warn("EPSS: %v", err)
	}

	return e.Each(ctx, x, ids, func(_ int, id string, fields map[string]any) {
		fields["id"] = id
		if s, ok := scores[id]; ok {
			fields["epss"] = s.EPSS
			fields["epss_percentile"] = s.Percentile
		}

		if _, err := x.Node(ctx, "cve", fields); err != nil {
			x.Warn("%s: %v", id, err)

			return
		}

		x.Info("%s: %v, CVSS %v, EPSS %v", id, fields["severity"], fields["cvss"], fields["epss"])
	})
}

// CVEToVulnerability makes a vulnerability of a CVE: its severity,
// CVSS, CWE, description and references, open, referencing the CVE.
// affected says where it is, so one CVE on two systems is two
// vulnerabilities. Run again on a vulnerability that exists, it fills
// only what is empty: the status and the assessment are the
// investigator's.
type CVEToVulnerability struct{}

// Meta describes the transform.
func (CVEToVulnerability) Meta() transform.Meta {
	return transform.Meta{Name: "cve_to_vulnerability", Category: "Security", Input: "cve", Output: "vulnerability",
		Description: "Make a vulnerability of a CVE, with its severity, CVSS, CWE, description and references, open and linked to the CVE.",
		Params: []transform.Param{{Name: "affected", Type: transform.ParamString,
			Description: "What it affects, a URL, a host or a component and version. Empty leaves it to fill in."}}}
}

// Run makes one vulnerability per CVE.
func (CVEToVulnerability) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	affected := strings.TrimSpace(x.Param("affected"))
	for _, in := range inputs {
		id := props.Str(in.Properties, "id")
		v := existing(ctx, x, in, affected)
		if v == nil {
			v = map[string]any{"name": id, "cve": id, "status": "open"}
			if affected != "" {
				v["affected"] = affected
			}
		}

		for _, k := range nvd.Shared {
			if !types.Truthy(v[k]) && types.Truthy(in.Properties[k]) {
				v[k] = in.Properties[k]
			}
		}

		n, err := x.Node(ctx, "vulnerability", v)
		if err != nil {
			x.Warn("%s: %v", id, err)

			continue
		}

		if err := x.Link(ctx, n, in, "REFERENCES"); err != nil {
			x.Warn("%s: %v", id, err)
		}
	}

	return nil
}

// existing is the vulnerability already drawn from cve at affected, as
// its properties, nil when there is none.
func existing(ctx context.Context, x *transform.Exec, cve graphmodel.Node, affected string) map[string]any {
	nodes, _, err := x.Neighbours(ctx, cve)
	if err != nil {
		return nil
	}

	for _, n := range nodes {
		if n.Type == "vulnerability" && props.Str(n.Properties, "cve") == props.Str(cve.Properties, "id") &&
			props.Str(n.Properties, "affected") == affected {
			return maps.Clone(n.Properties)
		}
	}

	return nil
}

// All is the package's transforms, pointed at the NVD.
func All() []transform.Transform {
	return []transform.Transform{CVEToDetails{Client: nvd.Public, EPSS: epss.Public}, CVEToVulnerability{}}
}
