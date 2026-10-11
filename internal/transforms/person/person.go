// Gosint, Copyright (c) 2026 YouSysAdmin

// Package person holds the transforms that start from an individual.
package person

import (
	"context"
	"net/url"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/whoxy"
)

// IndividualToDomains finds the domains registered in a person's name.
type IndividualToDomains struct{ whoxy.Client }

// Meta describes the transform.
func (IndividualToDomains) Meta() transform.Meta {
	return transform.Meta{Name: "individual_to_domains", Category: "Individual", Input: "individual", Output: "domain",
		Description: "Find the domains registered in a person's name (Whoxy reverse WHOIS).",
		Params:      []transform.Param{whoxy.Key}}
}

// Run enriches each person.
func (e IndividualToDomains) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.Reverse(ctx, x, in, url.Values{"name": {in.Label}}, "HAS_REGISTERED_DOMAIN")
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		IndividualToDomains{Client: whoxy.Public},
		IndividualToDehashed{Client: dehashed.Public},
	}
}
