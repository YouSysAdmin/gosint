// Gosint, Copyright (c) 2026 YouSysAdmin

package ip

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// IPToDehashed finds the breach records of an IP address in DeHashed and what leaked with it.
type IPToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (IPToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_dehashed", Category: "Ip", Input: "ip", Output: "breach",
		Description: "Find the breach records of an IP address in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e IPToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "ip_address", func(n graphmodel.Node) string { return props.Str(n.Properties, "address") })
}
