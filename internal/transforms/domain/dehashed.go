// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// DomainToDehashed finds the breach records of a domain, every account under it in DeHashed and what leaked with it.
type DomainToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (DomainToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_dehashed", Category: "Domain", Input: "domain", Output: "breach",
		Description: "Find the breach records of a domain, every account under it in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e DomainToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "domain", func(n graphmodel.Node) string { return props.Str(n.Properties, "domain") })
}
