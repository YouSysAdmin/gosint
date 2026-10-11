// Gosint, Copyright (c) 2026 YouSysAdmin

// Package org holds the transforms that start from an organization.
package org

import (
	"context"
	"net/url"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/ripestat"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/whoxy"
)

// OrgToDomains finds the domains registered to a company.
type OrgToDomains struct{ whoxy.Client }

// Meta describes the transform.
func (OrgToDomains) Meta() transform.Meta {
	return transform.Meta{Name: "org_to_domains", Category: "Organization", Input: "organization", Output: "domain",
		Description: "Find the domains registered to an organization (Whoxy reverse WHOIS).",
		Params:      []transform.Param{whoxy.Key}}
}

// Run enriches each organization.
func (e OrgToDomains) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.Reverse(ctx, x, in, url.Values{"company": {in.Label}}, "HAS_DOMAIN")
	}

	return nil
}

// OrgToASN links an organization to the autonomous systems registered
// under its name.
type OrgToASN struct{ ripestat.Client }

// Meta describes the transform.
func (OrgToASN) Meta() transform.Meta {
	return transform.Meta{Name: "org_to_asn", Category: "Organization", Input: "organization", Output: "asn",
		Description: "Find autonomous systems registered under an organization's name (RIPEstat)."}
}

// Run enriches each organization, linking each to its own matches.
func (e OrgToASN) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		name := props.Str(in.Properties, "name")
		if name == "" {
			name = in.Label
		}

		var search struct {
			Categories []struct {
				Category    string `json:"category"`
				Suggestions []struct {
					Value       string `json:"value"`
					Description string `json:"description"`
				} `json:"suggestions"`
			} `json:"categories"`
		}

		if err := e.Get(ctx, x, "searchcomplete", name, &search); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, cat := range search.Categories {
			if cat.Category != "ASNs" {
				continue
			}

			for _, s := range cat.Suggestions {
				as := map[string]any{"asn_str": s.Value, "name": s.Description, "description": s.Description}
				if _, err := x.NodeAndLink(ctx, in, "BELONGS_TO", "asn", as); err != nil {
					x.Warn("%s: %s: %v", in.Label, s.Value, err)
				}
			}
		}
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		OrgToDomains{Client: whoxy.Public},
		OrgToASN{Client: ripestat.Public},
	}
}
