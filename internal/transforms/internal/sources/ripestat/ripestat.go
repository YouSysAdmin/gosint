// Gosint, Copyright (c) 2026 YouSysAdmin

// Package ripestat is the RIPEstat data API, which answers ASN
// questions without a key.
package ripestat

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the public instance.
const URL = "https://stat.ripe.net/data/"

// Public is the client of the public instance.
var Public = Client{BaseURL: URL}

// Client talks to one RIPEstat.
type Client struct {
	BaseURL string
}

// Get asks a data call about resource and reads its data into out.
func (c Client) Get(ctx context.Context, x *transform.Exec, call, resource string, out any) error {
	var envelope struct {
		Status string `json:"status"`
		Data   any    `json:"data"`
	}

	envelope.Data = out
	if _, err := httpx.GetJSON(ctx, x, c.BaseURL+call+"/data.json?resource="+url.QueryEscape(resource), nil, &envelope); err != nil {
		return err
	}

	if envelope.Status != "" && envelope.Status != "ok" {
		return fmt.Errorf("RIPEstat %s answered %s", call, envelope.Status)
	}

	return nil
}

// ASProps are the properties of AS number n: its holder and country.
func (c Client) ASProps(ctx context.Context, x *transform.Exec, n string) map[string]any {
	asn := "AS" + strings.TrimPrefix(strings.ToUpper(n), "AS")
	props := map[string]any{"asn_str": asn}

	var overview struct {
		Holder string `json:"holder"`
	}

	if err := c.Get(ctx, x, "as-overview", asn, &overview); err == nil && overview.Holder != "" {
		props["name"] = overview.Holder
		props["description"] = overview.Holder
	}

	var country struct {
		Located []struct {
			Location string `json:"location"`
		} `json:"located_resources"`
	}

	if err := c.Get(ctx, x, "rir-stats-country", asn, &country); err == nil && len(country.Located) > 0 {
		props["country"] = country.Located[0].Location
	}

	return props
}

// ASNsOfIP lists the origin ASNs of an address.
func (c Client) ASNsOfIP(ctx context.Context, x *transform.Exec, ip string) ([]string, error) {
	var info struct {
		ASNs []string `json:"asns"`
	}

	err := c.Get(ctx, x, "network-info", ip, &info)

	return info.ASNs, err
}

// LinkASNs links from to the autonomous systems announcing ip.
func (c Client) LinkASNs(ctx context.Context, x *transform.Exec, from graphmodel.Node, ip, label string) {
	asns, err := c.ASNsOfIP(ctx, x, ip)
	if err != nil {
		x.Warn("%s: %v", from.Label, err)

		return
	}

	for _, n := range asns {
		if _, err := x.NodeAndLink(ctx, from, label, "asn", c.ASProps(ctx, x, n)); err != nil {
			x.Warn("%s: AS%s: %v", from.Label, n, err)
		}
	}
}
