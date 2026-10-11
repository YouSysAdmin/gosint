// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/ripestat"
)

// DomainToASN links a domain to the AS hosting its addresses.
type DomainToASN struct{ ripestat.Client }

// Meta describes the transform.
func (DomainToASN) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_asn", Category: "Domain", Input: "domain", Output: "asn",
		Description: "Find the autonomous systems hosting a domain's addresses (RIPEstat)."}
}

// Run enriches each domain.
func (e DomainToASN) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addrs, err := x.Resolver.LookupIPAddr(ctx, props.Str(in.Properties, "domain"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, a := range addrs {
			e.LinkASNs(ctx, x, in, a.IP.String(), "HOSTED_IN")
		}
	}

	return nil
}
