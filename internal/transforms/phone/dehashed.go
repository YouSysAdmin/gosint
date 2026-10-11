// Gosint, Copyright (c) 2026 YouSysAdmin

package phone

import (
	"context"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// PhoneToDehashed finds the breach records of a phone number in DeHashed and what leaked with it.
type PhoneToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (PhoneToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "phone_to_dehashed", Category: "Phone", Input: "phone", Output: "breach",
		Description: "Find the breach records of a phone number in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e PhoneToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "phone", func(n graphmodel.Node) string { return digits(props.Str(n.Properties, "number")) })
}

// digits is the number as DeHashed stores it, digits only.
func digits(s string) string {
	var b []rune
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b = append(b, r)
		}
	}

	return string(b)
}
