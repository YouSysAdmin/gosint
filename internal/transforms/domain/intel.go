// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// The intelligence sources on a domain: what a service knows about it,
// laid over the domain or drawn around it.

// The services' default URLs.
const (
	URLScanURL        = "https://urlscan.io"
	VirusTotalURL     = "https://www.virustotal.com"
	SecurityTrailsURL = "https://api.securitytrails.com"
	HunterURL         = "https://api.hunter.io"
)

// DomainToURLScan lists the pages urlscan.io scanned under a domain.
type DomainToURLScan struct {
	BaseURL string
}

// Meta describes the transform.
func (DomainToURLScan) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_urlscan", Category: "Domain", Input: "domain", Output: "website",
		Description: "List the pages urlscan.io scanned under the domain, each with the address it was served from.",
		Params: []transform.Param{
			{Name: "URLSCAN_API_KEY", Type: transform.ParamSecret, Required: false, Description: "An urlscan.io API key, for more results and private scans."},
			{Name: "max_results", Type: transform.ParamNumber, Default: "100", Description: "How many scans to read."},
		}}
}

// Run enriches each domain.
func (e DomainToURLScan) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"Accept": {"application/json"}}
	if key := x.Param("URLSCAN_API_KEY"); key != "" {
		header.Set("API-Key", key)
	}

	size, _ := strconv.Atoi(x.Param("max_results"))
	if size <= 0 || size > 1000 {
		size = 100
	}

	for _, in := range inputs {
		domain := props.Str(in.Properties, "domain")

		var answer struct {
			Results []struct {
				Page struct {
					URL     string `json:"url"`
					Domain  string `json:"domain"`
					IP      string `json:"ip"`
					Title   string `json:"title"`
					Country string `json:"country"`
				} `json:"page"`
				Task struct {
					Time string `json:"time"`
				} `json:"task"`
			} `json:"results"`
		}

		q := url.Values{"q": {"domain:" + domain}, "size": {strconv.Itoa(size)}}
		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v1/search/?"+q.Encode(), header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		seen := map[string]bool{}
		for _, r := range answer.Results {
			if r.Page.URL == "" || seen[r.Page.URL] {
				continue
			}

			seen[r.Page.URL] = true
			site := map[string]any{"url": r.Page.URL, "scanned_at": r.Task.Time}
			if r.Page.Title != "" {
				site["title"] = r.Page.Title
			}

			w, err := x.NodeAndLink(ctx, in, "HAS_WEBSITE", "website", site)
			if err != nil {
				x.Warn("%s: %s: %v", in.Label, r.Page.URL, err)

				continue
			}

			if r.Page.IP != "" {
				addr := map[string]any{"address": r.Page.IP}
				if r.Page.Country != "" {
					addr["country"] = r.Page.Country
				}

				if _, err := x.NodeAndLink(ctx, w, "RESOLVES_TO", "ip", addr); err != nil {
					x.Warn("%s: %s: %v", in.Label, r.Page.IP, err)
				}
			}
		}

		x.Info("%s: %d scanned pages", in.Label, len(seen))
	}

	return nil
}

// DomainToVirusTotal reads VirusTotal's verdicts on a domain and the
// addresses it resolved to.
type DomainToVirusTotal struct {
	BaseURL string
}

// Meta describes the transform.
func (DomainToVirusTotal) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_virustotal", Category: "Domain", Input: "domain", Output: "ip",
		Description: "Read how many engines call a domain malicious on VirusTotal (any red) and the addresses it resolved to.",
		Params: []transform.Param{
			{Name: "VIRUSTOTAL_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The VirusTotal API key."},
		}}
}

// Run enriches each domain.
func (e DomainToVirusTotal) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"x-apikey": {x.Param("VIRUSTOTAL_API_KEY")}}
	for _, in := range inputs {
		domain := url.PathEscape(props.Str(in.Properties, "domain"))

		var answer struct {
			Data struct {
				Attributes struct {
					Stats struct {
						Malicious  int `json:"malicious"`
						Suspicious int `json:"suspicious"`
						Harmless   int `json:"harmless"`
						Undetected int `json:"undetected"`
					} `json:"last_analysis_stats"`
					Reputation int               `json:"reputation"`
					Categories map[string]string `json:"categories"`
				} `json:"attributes"`
			} `json:"data"`
		}

		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v3/domains/"+domain, header, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		a := answer.Data.Attributes
		merged := maps.Clone(in.Properties)
		merged["vt_malicious"], merged["vt_suspicious"], merged["vt_harmless"] = a.Stats.Malicious, a.Stats.Suspicious, a.Stats.Harmless
		merged["vt_undetected"], merged["vt_reputation"] = a.Stats.Undetected, a.Reputation
		cats := map[string]bool{}
		for _, c := range a.Categories {
			cats[c] = true
		}

		if len(cats) > 0 {
			list := make([]any, 0, len(cats))
			for c := range cats {
				list = append(list, c)
			}

			merged["vt_categories"] = list
		}

		n, err := x.Node(ctx, in.Type, merged)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if a.Stats.Malicious > 0 {
			_ = x.Annotate(ctx, n, nil, "red")
		} else {
			_ = x.Unflag(ctx, in, "red")
		}

		x.Found("%s: %d malicious, %d suspicious", in.Label, a.Stats.Malicious, a.Stats.Suspicious)

		var resolutions struct {
			Data []struct {
				Attributes struct {
					IP   string `json:"ip_address"`
					Date int64  `json:"date"`
				} `json:"attributes"`
			} `json:"data"`
		}

		if found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/api/v3/domains/"+domain+"/resolutions?limit=40", header, &resolutions); err != nil || !found {
			continue
		}

		for _, r := range resolutions.Data {
			if r.Attributes.IP == "" {
				continue
			}

			addr, err := x.NodeFrom(ctx, in, "ip", map[string]any{"address": r.Attributes.IP})
			if err != nil {
				x.Warn("%s: %s: %v", in.Label, r.Attributes.IP, err)

				continue
			}

			if err := x.LinkWith(ctx, in, addr, "RESOLVED_TO", map[string]any{"at": r.Attributes.Date}); err != nil {
				x.Warn("%s: %s: %v", in.Label, r.Attributes.IP, err)
			}
		}
	}

	return nil
}

// DomainToSecurityTrails lists a domain's subdomains and current
// addresses from SecurityTrails.
type DomainToSecurityTrails struct {
	BaseURL string
}

// Meta describes the transform.
func (DomainToSecurityTrails) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_securitytrails", Category: "Domain", Input: "domain", Output: "domain",
		Description: "List a domain's subdomains and the addresses its A records point to, from SecurityTrails.",
		Params: []transform.Param{
			{Name: "SECURITYTRAILS_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The SecurityTrails API key."},
			{Name: "max_results", Type: transform.ParamNumber, Default: "200", Description: "How many subdomains to draw."},
		}}
}

// Run enriches each domain.
func (e DomainToSecurityTrails) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"APIKEY": {x.Param("SECURITYTRAILS_API_KEY")}, "Accept": {"application/json"}}
	limit, _ := strconv.Atoi(x.Param("max_results"))
	if limit <= 0 {
		limit = 200
	}

	for _, in := range inputs {
		domain := props.Str(in.Properties, "domain")

		var subs struct {
			Subdomains []string `json:"subdomains"`
		}

		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/v1/domain/"+url.PathEscape(domain)+"/subdomains?children_only=false", header, &subs)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if found {
			for i, s := range subs.Subdomains {
				if i == limit {
					x.Info("%s: %d more subdomains not drawn, raise max_results", in.Label, len(subs.Subdomains)-limit)

					break
				}

				if _, err := x.NodeAndLink(ctx, in, "HAS_SUBDOMAIN", "domain", map[string]any{"domain": s + "." + domain}); err != nil {
					x.Warn("%s: %s: %v", in.Label, s, err)
				}
			}
		}

		var current struct {
			DNS struct {
				A struct {
					Values []struct {
						IP string `json:"ip"`
					} `json:"values"`
				} `json:"a"`
			} `json:"current_dns"`
		}

		if found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/v1/domain/"+url.PathEscape(domain), header, &current); err != nil || !found {
			continue
		}

		for _, v := range current.DNS.A.Values {
			if _, err := x.NodeAndLink(ctx, in, "RESOLVES_TO", "ip", map[string]any{"address": v.IP}); err != nil {
				x.Warn("%s: %s: %v", in.Label, v.IP, err)
			}
		}
	}

	return nil
}

// DomainToHunter lists the email addresses Hunter.io found on a domain,
// with the people they belong to.
type DomainToHunter struct {
	BaseURL string
}

// Meta describes the transform.
func (DomainToHunter) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_hunter", Category: "Domain", Input: "domain", Output: "email",
		Description: "List the email addresses Hunter.io found on a domain, each under the person it names when known.",
		Params: []transform.Param{
			{Name: "HUNTER_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The Hunter.io API key."},
			{Name: "max_results", Type: transform.ParamNumber, Default: "100", Description: "How many addresses to read."},
		}}
}

// Run enriches each domain.
func (e DomainToHunter) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	limit, _ := strconv.Atoi(x.Param("max_results"))
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	for _, in := range inputs {
		var answer struct {
			Data struct {
				Organization string `json:"organization"`
				Emails       []struct {
					Value      string `json:"value"`
					Type       string `json:"type"`
					Confidence int    `json:"confidence"`
					FirstName  string `json:"first_name"`
					LastName   string `json:"last_name"`
					Position   string `json:"position"`
				} `json:"emails"`
			} `json:"data"`
		}

		q := url.Values{"domain": {props.Str(in.Properties, "domain")}, "api_key": {x.Param("HUNTER_API_KEY")}, "limit": {strconv.Itoa(limit)}}
		found, err := httpx.GetJSON(ctx, x, e.BaseURL+"/v2/domain-search?"+q.Encode(), nil, &answer)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		for _, m := range answer.Data.Emails {
			email := map[string]any{"email": m.Value, "hunter_confidence": m.Confidence, "hunter_type": m.Type}
			node, err := x.NodeAndLink(ctx, in, "HAS_EMAIL", "email", email)
			if err != nil {
				x.Warn("%s: %s: %v", in.Label, m.Value, err)

				continue
			}

			name := strings.TrimSpace(m.FirstName + " " + m.LastName)
			if name == "" {
				continue
			}

			person := map[string]any{"full_name": name, "first_name": m.FirstName, "last_name": m.LastName}
			if m.Position != "" {
				person["job_title"] = m.Position
			}

			p, err := x.NodeFrom(ctx, in, "individual", person)
			if err != nil {
				x.Warn("%s: %s: %v", in.Label, name, err)

				continue
			}

			if err := x.Link(ctx, p, node, "HAS_EMAIL"); err != nil {
				x.Warn("%s: %s: %v", in.Label, name, err)
			}
		}

		x.Info("%s: %d addresses from Hunter", in.Label, len(answer.Data.Emails))
	}

	return nil
}
