// Gosint, Copyright (c) 2026 YouSysAdmin

// Package cidr holds the transforms that start from an IP range.
package cidr

import (
	"context"
	"net/netip"
	"strconv"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// maxHosts bounds how many addresses one range expands into, so a /8
// does not put sixteen million nodes on a sketch.
const maxHosts = 65536

// CIDRToIPs expands a range into its addresses.
type CIDRToIPs struct{}

// Meta describes the transform.
func (CIDRToIPs) Meta() transform.Meta {
	return transform.Meta{Name: "cidr_to_ips", Category: "Cidr", Input: "cidr", Output: "ip",
		Description: "List the addresses of an IP range, up to a limit.",
		Params: []transform.Param{{Name: "limit", Type: transform.ParamNumber, Default: "256",
			Description: "Most addresses to add per range, at most " + strconv.Itoa(maxHosts)}}}
}

// Run enriches each range.
func (CIDRToIPs) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	limit, _ := strconv.Atoi(x.Param("limit"))
	limit = min(max(limit, 1), maxHosts)

	for _, in := range inputs {
		prefix, err := netip.ParsePrefix(props.Str(in.Properties, "network"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		added := 0
		a := prefix.Masked().Addr()
		for ; a.IsValid() && prefix.Contains(a) && added < limit; a = a.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}

			if _, err := x.NodeAndLink(ctx, in, "CONTAINS", "ip", map[string]any{"address": a.String()}); err != nil {
				x.Warn("%s: %v", in.Label, err)

				continue
			}

			added++
		}

		if a.IsValid() && prefix.Contains(a) {
			x.Warn("%s: stopped at %d addresses, raise limit to add more", in.Label, limit)
		}
	}

	return nil
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{CIDRToIPs{}}
}
