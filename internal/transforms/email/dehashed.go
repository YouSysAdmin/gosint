// Gosint, Copyright (c) 2026 YouSysAdmin

package email

import (
	"context"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
)

// EmailToDehashed finds the breach records of an email address in DeHashed and what leaked with it.
type EmailToDehashed struct{ dehashed.Client }

// Meta describes the transform.
func (EmailToDehashed) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_dehashed", Category: "Email", Input: "email", Output: "breach",
		Description: "Find the breach records of an email address in DeHashed and what leaked with it: the password or its hash, names, emails, usernames, phones, IPs.",
		Params:      dehashed.Params}
}

// Run enriches each input.
func (e EmailToDehashed) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	return e.Client.Run(ctx, x, inputs, "email", func(n graphmodel.Node) string { return strings.ToLower(props.Str(n.Properties, "email")) })
}
