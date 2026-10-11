// Gosint, Copyright (c) 2026 YouSysAdmin

package username

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
)

// UsernameToInfostealers finds the computers an infostealer took a
// login with the username from (HudsonRock).
type UsernameToInfostealers struct{ hudsonrock.Client }

// Meta describes the transform.
func (UsernameToInfostealers) Meta() transform.Meta {
	return transform.Meta{Name: "username_to_infostealers", Category: "Social", Input: "username", Output: "device",
		Description: "Find computers infected by an infostealer that held a login with this username, with the stealer and what the free API shows (HudsonRock)."}
}

// Run enriches each username.
func (e UsernameToInfostealers) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.Lookup(ctx, x, in, "username", props.Str(in.Properties, "value"))
	}

	return nil
}
