// Gosint, Copyright (c) 2026 YouSysAdmin

// Package phone holds the transforms that start from a phone number.
package phone

import (
	"context"
	"net/url"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
)

// VeriphoneURL is Veriphone's verify endpoint.
const VeriphoneURL = "https://api.veriphone.io/v2/verify"

// PhoneToCarrier adds the carrier of a phone number (Veriphone).
type PhoneToCarrier struct {
	BaseURL string
}

// Meta describes the transform.
func (PhoneToCarrier) Meta() transform.Meta {
	return transform.Meta{Name: "phone_to_carrier", Category: "Phone", Input: "phone", Output: "phone",
		Description: "Find the carrier and country of a phone number (Veriphone).",
		Params: []transform.Param{{Name: "VERIPHONE_API_KEY", Type: transform.ParamSecret, Required: true,
			Description: "The Veriphone API key for phone number carrier lookups."}}}
}

// Run enriches each number in place.
func (e PhoneToCarrier) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		number := props.Str(in.Properties, "number")

		var answer struct {
			Carrier string `json:"carrier"`
			Country string `json:"country"`
		}

		q := url.Values{"key": {x.Param("VERIPHONE_API_KEY")}, "phone": {number}}
		if _, err := httpx.GetJSON(ctx, x, e.BaseURL+"?"+q.Encode(), nil, &answer); err != nil {
			x.Warn("%s: %v", number, err)

			continue
		}

		if answer.Carrier == "" {
			continue
		}

		p := map[string]any{"number": number, "carrier": answer.Carrier}
		if answer.Country != "" {
			p["country"] = answer.Country
		}

		if _, err := x.Node(ctx, "phone", p); err != nil {
			x.Warn("%s: %v", number, err)

			continue
		}

		x.Found("carrier of %s is %s", number, answer.Carrier)
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{PhoneToCarrier{BaseURL: VeriphoneURL}, PhoneToInfostealers{Client: hudsonrock.Public}, PhoneToDehashed{Client: dehashed.Public}}
}
