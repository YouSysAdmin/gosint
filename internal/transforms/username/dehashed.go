// Gosint, Copyright (c) 2026 YouSysAdmin

package username

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// UsernameToDehashed finds the breach records of a username in DeHashed and what leaked with it.
type UsernameToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (UsernameToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "username_to_dehashed", Category: "Social", Input: "username", Output: "breach",
		Description: "Find the breach records of a username in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e UsernameToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "username", func(n graphmodel.Node) string { return props.Str(n.Properties, "value") })
}
