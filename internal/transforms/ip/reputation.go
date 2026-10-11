// Gosint, Copyright (c) 2026 YouSysAdmin

package ip

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// The reputation sources: each reads what a service knows about an
// address and lays it over the address, flagging one the service calls
// bad, so a sketch full of addresses shows the ones to look at.

// The services' default URLs.
const (
	AbuseIPDBURL  = "https://api.abuseipdb.com"
	GreyNoiseURL  = "https://api.greynoise.io"
	VirusTotalURL = "https://www.virustotal.com"
	CensysURL     = "https://search.censys.io"
	ShodanURL     = "https://api.shodan.io"
)

// layOver lays extra over the input's properties and writes it back,
// leaving its flag alone.
func layOver(ctx context.Context, x *transform.Exec, in graphmodel.Node, extra map[string]any) (graphmodel.Node, error) {
	merged := maps.Clone(in.Properties)
	if merged == nil {
		merged = map[string]any{}
	}

	maps.Copy(merged, extra)

	return x.Node(ctx, in.Type, merged)
}

// enrich lays extra over the input like layOver, flagging it red when
// bad says so and clearing an earlier red flag when it does not.
func enrich(ctx context.Context, x *transform.Exec, in graphmodel.Node, extra map[string]any, bad bool) error {
	n, err := layOver(ctx, x, in, extra)
	if err != nil {
		return err
	}

	switch {
	case bad:
		return x.Annotate(ctx, n, nil, "red")
	default:
		return x.Unflag(ctx, in, "red")
	}
}

// IPToAbuseIPDB reads an address's abuse reports from AbuseIPDB.
type IPToAbuseIPDB struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToAbuseIPDB) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_abuseipdb", Category: "IP", Input: "ip", Output: "ip",
		Description: "Read an address's abuse confidence and reports from AbuseIPDB, red from 75 up.",
		Params: []transform.Param{
			{Name: "ABUSEIPDB_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The AbuseIPDB API key."},
			{Name: "max_age_days", Type: transform.ParamNumber, Default: "90", Description: "How far back the reports count."},
		}}
}

// Run enriches each address.
func (e IPToAbuseIPDB) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"Key": {x.Param("ABUSEIPDB_API_KEY")}, "Accept": {"application/json"}}
	for _, in := range inputs {
		var answer struct {
			Data struct {
				Score     int    `json:"abuseConfidenceScore"`
				Reports   int    `json:"totalReports"`
				Country   string `json:"countryCode"`
				ISP       string `json:"isp"`
				Domain    string `json:"domain"`
				UsageType string `json:"usageType"`
				Tor       bool   `json:"isTor"`
				LastAt    string `json:"lastReportedAt"`
			} `json:"data"`
		}

		q := url.Values{"ipAddress": {props.Str(in.Properties, "address")}, "maxAgeInDays": {x.Param("max_age_days")}}
		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v2/check?"+q.Encode(), header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		d := answer.Data
		extra := map[string]any{"abuse_score": d.Score, "abuse_reports": d.Reports, "usage_type": d.UsageType, "is_tor": d.Tor}
		if d.LastAt != "" {
			extra["last_reported_at"] = d.LastAt
		}

		if props.Str(in.Properties, "country") == "" && d.Country != "" {
			extra["country"] = d.Country
		}

		if props.Str(in.Properties, "isp") == "" && d.ISP != "" {
			extra["isp"] = d.ISP
		}

		if err := enrich(ctx, x, in, extra, d.Score >= 75); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		x.Found("%s: abuse score %d, %d reports", in.Label, d.Score, d.Reports)
	}

	return nil
}

// IPToGreyNoise asks GreyNoise's community API whether an address is
// internet background noise or a known good service.
type IPToGreyNoise struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToGreyNoise) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_greynoise", Category: "IP", Input: "ip", Output: "ip",
		Description: "Ask GreyNoise whether an address scans the internet (noise) or is a known service (riot), malicious ones red.",
		Params: []transform.Param{
			{Name: "GREYNOISE_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The GreyNoise API key."},
		}}
}

// Run enriches each address.
func (e IPToGreyNoise) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"key": {x.Param("GREYNOISE_API_KEY")}, "Accept": {"application/json"}}
	for _, in := range inputs {
		var answer struct {
			Noise          bool   `json:"noise"`
			Riot           bool   `json:"riot"`
			Classification string `json:"classification"`
			Name           string `json:"name"`
			LastSeen       string `json:"last_seen"`
			Message        string `json:"message"`
		}

		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/v3/community/"+url.PathEscape(props.Str(in.Properties, "address")), header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			x.Info("%s: GreyNoise has not seen it", in.Label)

			continue
		}

		extra := map[string]any{"greynoise_noise": answer.Noise, "greynoise_riot": answer.Riot,
			"greynoise_classification": answer.Classification, "greynoise_name": answer.Name}
		if answer.LastSeen != "" {
			extra["greynoise_last_seen"] = answer.LastSeen
		}

		if err := enrich(ctx, x, in, extra, answer.Classification == "malicious"); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		x.Found("%s: %s, noise %t, riot %t", in.Label, answer.Classification, answer.Noise, answer.Riot)
	}

	return nil
}

// vtStats is VirusTotal's verdict count.
type vtStats struct {
	Malicious  int `json:"malicious"`
	Suspicious int `json:"suspicious"`
	Harmless   int `json:"harmless"`
	Undetected int `json:"undetected"`
}

// vtExtra is what a VirusTotal object lays over an entity.
func vtExtra(stats vtStats, reputation int) map[string]any {
	return map[string]any{"vt_malicious": stats.Malicious, "vt_suspicious": stats.Suspicious,
		"vt_harmless": stats.Harmless, "vt_undetected": stats.Undetected, "vt_reputation": reputation}
}

// IPToVirusTotal reads VirusTotal's verdicts on an address.
type IPToVirusTotal struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToVirusTotal) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_virustotal", Category: "IP", Input: "ip", Output: "ip",
		Description: "Read how many engines call an address malicious on VirusTotal, any red.",
		Params: []transform.Param{
			{Name: "VIRUSTOTAL_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The VirusTotal API key."},
		}}
}

// Run enriches each address.
func (e IPToVirusTotal) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"x-apikey": {x.Param("VIRUSTOTAL_API_KEY")}}
	for _, in := range inputs {
		var answer struct {
			Data struct {
				Attributes struct {
					Stats      vtStats `json:"last_analysis_stats"`
					Reputation int     `json:"reputation"`
					Owner      string  `json:"as_owner"`
					Country    string  `json:"country"`
				} `json:"attributes"`
			} `json:"data"`
		}

		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v3/ip_addresses/"+url.PathEscape(props.Str(in.Properties, "address")), header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		a := answer.Data.Attributes
		extra := vtExtra(a.Stats, a.Reputation)
		if props.Str(in.Properties, "isp") == "" && a.Owner != "" {
			extra["isp"] = a.Owner
		}

		if props.Str(in.Properties, "country") == "" && a.Country != "" {
			extra["country"] = a.Country
		}

		if err := enrich(ctx, x, in, extra, a.Stats.Malicious > 0); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		x.Found("%s: %d malicious, %d suspicious", in.Label, a.Stats.Malicious, a.Stats.Suspicious)
	}

	return nil
}

// IPToCensys reads the services Censys saw on an address.
type IPToCensys struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToCensys) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_censys", Category: "IP", Input: "ip", Output: "port",
		Description: "List the services Censys saw on an address as ports, with its location and network.",
		Params: []transform.Param{
			{Name: "CENSYS_API_ID", Type: transform.ParamSecret, Required: true, Description: "The Censys API ID."},
			{Name: "CENSYS_API_SECRET", Type: transform.ParamSecret, Required: true, Description: "The Censys API secret."},
		}}
}

// Run enriches each address.
func (e IPToCensys) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	auth := base64.StdEncoding.EncodeToString([]byte(x.Param("CENSYS_API_ID") + ":" + x.Param("CENSYS_API_SECRET")))
	header := http.Header{"Authorization": {"Basic " + auth}, "Accept": {"application/json"}}
	for _, in := range inputs {
		var answer struct {
			Result struct {
				Services []struct {
					Port      int    `json:"port"`
					Name      string `json:"service_name"`
					Transport string `json:"transport_protocol"`
				} `json:"services"`
				Location struct {
					Country string `json:"country"`
					City    string `json:"city"`
				} `json:"location"`
				AS struct {
					ASN  int    `json:"asn"`
					Name string `json:"name"`
				} `json:"autonomous_system"`
			} `json:"result"`
		}

		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v2/hosts/"+url.PathEscape(props.Str(in.Properties, "address")), header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			x.Info("%s: Censys has not seen it", in.Label)

			continue
		}

		r := answer.Result
		extra := map[string]any{}
		if props.Str(in.Properties, "country") == "" && r.Location.Country != "" {
			extra["country"] = r.Location.Country
		}

		if props.Str(in.Properties, "city") == "" && r.Location.City != "" {
			extra["city"] = r.Location.City
		}

		if r.AS.ASN != 0 {
			extra["asn"] = fmt.Sprintf("AS%d", r.AS.ASN)
			if props.Str(in.Properties, "isp") == "" {
				extra["isp"] = r.AS.Name
			}
		}

		if len(extra) > 0 {
			if _, err := layOver(ctx, x, in, extra); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}

		for _, s := range r.Services {
			port := map[string]any{"number": s.Port, "protocol": s.Transport, "state": "open", "service": strings.ToLower(s.Name)}
			if _, err := x.NodeAndLink(ctx, in, "HAS_PORT", "port", port); err != nil {
				x.Warn("%s: port %d: %v", in.Label, s.Port, err)
			}
		}
	}

	return nil
}

// shodanBannerMax bounds a banner kept on a port.
const shodanBannerMax = 1000

// IPToShodan reads what Shodan's host API saw on an address.
type IPToShodan struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToShodan) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_shodan", Category: "IP", Input: "ip", Output: "port",
		Description: "List the services Shodan saw on an address as ports with their banners, with its location, network, names and the CVEs Shodan matches.",
		Params: []transform.Param{
			{Name: "SHODAN_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The Shodan API key."},
		}}
}

// Run enriches each address. A refused key fails the scan, since every
// address would fail the same way.
func (e IPToShodan) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"Accept": {"application/json"}}
	key := url.QueryEscape(x.Param("SHODAN_API_KEY"))
	for _, in := range inputs {
		var answer struct {
			Country   string   `json:"country_name"`
			City      string   `json:"city"`
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			ISP       string   `json:"isp"`
			Org       string   `json:"org"`
			ASN       string   `json:"asn"`
			OS        string   `json:"os"`
			Hostnames []string `json:"hostnames"`
			Tags      []string `json:"tags"`
			Vulns     []string `json:"vulns"`
			Updated   string   `json:"last_update"`
			Data      []struct {
				Port      int    `json:"port"`
				Transport string `json:"transport"`
				Product   string `json:"product"`
				Version   string `json:"version"`
				Banner    string `json:"data"`
				Shodan    struct {
					Module string `json:"module"`
				} `json:"_shodan"`
			} `json:"data"`
		}

		u := e.BaseURL + "/shodan/host/" + url.PathEscape(props.Str(in.Properties, "address")) + "?key=" + key
		found, err := httpx.GetJSON(ctx, x, u, header, &answer)
		if err != nil {
			var se httpx.StatusError
			if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
				return errors.New("the Shodan key was refused: it is wrong or its plan has no API access")
			}

			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			x.Info("%s: Shodan has not seen it", in.Label)

			continue
		}

		extra := map[string]any{}
		fill := func(field string, v any) {
			if old := in.Properties[field]; old == nil || old == "" {
				extra[field] = v
			}
		}

		if answer.Country != "" {
			fill("country", answer.Country)
		}

		if answer.City != "" {
			fill("city", answer.City)
		}

		if answer.Latitude != nil && answer.Longitude != nil {
			fill("latitude", *answer.Latitude)
			fill("longitude", *answer.Longitude)
		}

		if isp := cmp.Or(answer.ISP, answer.Org); isp != "" {
			fill("isp", isp)
		}

		if answer.ASN != "" {
			extra["asn"] = answer.ASN
		}

		if answer.OS != "" {
			extra["shodan_os"] = answer.OS
		}

		if len(answer.Tags) > 0 {
			extra["shodan_tags"] = answer.Tags
		}

		if answer.Updated != "" {
			extra["shodan_last_update"] = answer.Updated
		}

		if len(extra) > 0 {
			if _, err := layOver(ctx, x, in, extra); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}

		for _, s := range answer.Data {
			port := map[string]any{"number": s.Port, "protocol": strings.ToUpper(cmp.Or(s.Transport, "tcp")), "state": "open"}
			if service := strings.TrimSpace(s.Product + " " + s.Version); service != "" {
				port["service"] = service
			} else if s.Shodan.Module != "" {
				port["service"] = s.Shodan.Module
			}

			if banner := strings.TrimSpace(s.Banner); banner != "" {
				port["banner"] = truncate(banner, shodanBannerMax)
			}

			if _, err := x.NodeAndLink(ctx, in, "HAS_PORT", "port", port); err != nil {
				x.Warn("%s: port %d: %v", in.Label, s.Port, err)
			}
		}

		for _, name := range answer.Hostnames {
			if _, err := x.NodeAndLink(ctx, in, "REVERSE_RESOLVES_TO", "domain", map[string]any{"domain": name}); err != nil {
				x.Warn("%s: %s: %v", in.Label, name, err)
			}
		}

		for _, id := range answer.Vulns {
			if _, err := x.NodeAndLink(ctx, in, "REFERENCES", "cve", map[string]any{"id": id}); err != nil {
				x.Warn("%s: %s: %v", in.Label, id, err)
			}
		}

		x.Found("%s: %d services, %d CVEs", in.Label, len(answer.Data), len(answer.Vulns))
	}

	return nil
}

// truncate cuts s to at most n bytes on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}

	return s[:n]
}
