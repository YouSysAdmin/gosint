// Gosint, Copyright (c) 2026 YouSysAdmin

package phone

import (
	"context"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
)

// PhoneToInfostealers finds the computers an infostealer took a login
// with the phone number from (HudsonRock). The API has no phone search,
// a number used as a login is found by the username search, written
// with and without its plus sign.
type PhoneToInfostealers struct{ hudsonrock.Client }

// Meta describes the transform.
func (PhoneToInfostealers) Meta() transform.Meta {
	return transform.Meta{Name: "phone_to_infostealers", Category: "Phone", Input: "phone", Output: "device",
		Description: "Find computers infected by an infostealer that held a login with this phone number, with the stealer and what the free API shows (HudsonRock)."}
}

// Run enriches each phone number.
func (e PhoneToInfostealers) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		number := props.Str(in.Properties, "number")
		e.Lookup(ctx, x, in, "username", number)
		if bare := strings.TrimPrefix(number, "+"); bare != number && bare != "" {
			e.Lookup(ctx, x, in, "username", bare)
		}
	}

	return nil
}
