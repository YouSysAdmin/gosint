// Gosint, Copyright (c) 2026 YouSysAdmin

package person

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// IndividualToDehashed finds the breach records of a person by full name in DeHashed and what leaked with it.
type IndividualToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (IndividualToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "individual_to_dehashed", Category: "Individual", Input: "individual", Output: "breach",
		Description: "Find the breach records of a person by full name in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e IndividualToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "name", func(n graphmodel.Node) string { return props.Str(n.Properties, "full_name") })
}
