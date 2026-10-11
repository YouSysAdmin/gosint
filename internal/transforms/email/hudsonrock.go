// Gosint, Copyright (c) 2026 YouSysAdmin

package email

import (
	"context"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
)

// EmailToInfostealers finds the computers an infostealer took a login
// for the email from (HudsonRock).
type EmailToInfostealers struct{ hudsonrock.Client }

// Meta describes the transform.
func (EmailToInfostealers) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_infostealers", Category: "Email", Input: "email", Output: "device",
		Description: "Find computers infected by an infostealer that held a login with this email address, with the stealer and what the free API shows (HudsonRock)."}
}

// Run enriches each email.
func (e EmailToInfostealers) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.Lookup(ctx, x, in, "email", strings.ToLower(props.Str(in.Properties, "email")))
	}

	return nil
}
