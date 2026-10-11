// Gosint, Copyright (c) 2026 YouSysAdmin

// Package asn holds the transforms that start from an autonomous system.
package asn

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/ripestat"
)

// ASNToCIDRs links an AS to the prefixes it announces.
type ASNToCIDRs struct{ ripestat.Client }

// Meta describes the transform.
func (ASNToCIDRs) Meta() transform.Meta {
	return transform.Meta{Name: "asn_to_cidrs", Category: "Asn", Input: "asn", Output: "cidr",
		Description: "List the IP ranges an autonomous system announces (RIPEstat)."}
}

// Run enriches each AS.
func (e ASNToCIDRs) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		var announced struct {
			Prefixes []struct {
				Prefix string `json:"prefix"`
			} `json:"prefixes"`
		}

		if err := e.Get(ctx, x, "announced-prefixes", props.Str(in.Properties, "asn_str"), &announced); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, p := range announced.Prefixes {
			if p.Prefix == "0.0.0.0/0" || p.Prefix == "::/0" {
				continue
			}

			if _, err := x.NodeAndLink(ctx, in, "ANNOUNCES", "cidr", map[string]any{"network": p.Prefix}); err != nil {
				x.Warn("%s: %s: %v", in.Label, p.Prefix, err)
			}
		}

		x.Info("%s announces %d prefixes", in.Label, len(announced.Prefixes))
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{ASNToCIDRs{Client: ripestat.Public}}
}
