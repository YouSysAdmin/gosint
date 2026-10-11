// Gosint, Copyright (c) 2026 YouSysAdmin

// Package ip holds the transforms that start from an IP address.
package ip

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/crtsh"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nmap"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/ripestat"
)

const (
	// IPAPIURL is plain http: ip-api.com serves TLS to paying users only.
	IPAPIURL = "http://ip-api.com/json/"

	// InternetDBURL is Shodan's free InternetDB. It answers what Shodan
	// has already seen on an address, so nothing is sent to the target.
	InternetDBURL = "https://internetdb.shodan.io/"

	scamalyticsHost = "api12"
)

// IPToDomain links an address to the names it reverse resolves to,
// falling back to certificates issued for it.
type IPToDomain struct {
	CrtshURL string
}

// Meta describes the transform.
func (IPToDomain) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_domain", Category: "Ip", Input: "ip", Output: "domain",
		Description: "Find domains for an IP address by reverse DNS, or else by certificate transparency."}
}

// Run enriches each address. Each one links only to its own names.
func (e IPToDomain) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")
		names, err := x.Resolver.LookupAddr(ctx, addr)
		if err != nil || len(names) == 0 {
			names = e.fromCertificates(ctx, x, addr)
		}

		for _, name := range names {
			name = strings.TrimSuffix(strings.ToLower(name), ".")
			if _, err := x.NodeAndLink(ctx, in, "REVERSE_RESOLVES_TO", "domain", map[string]any{"domain": name}); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}
	}

	return nil
}

// fromCertificates is the first certificate name crt.sh lists for addr.
func (e IPToDomain) fromCertificates(ctx context.Context, x *transform.Exec, addr string) []string {
	entries, err := crtsh.Query(ctx, x, e.CrtshURL, addr)
	if err != nil {
		x.Warn("%s: crt.sh: %v", addr, err)

		return nil
	}

	for _, entry := range entries[:min(len(entries), 15)] {
		for name := range strings.SplitSeq(entry.NameValue, "\n") {
			name = strings.TrimSpace(name)
			if name != "" && name != addr && !strings.Contains(name, "*") {
				return []string{name}
			}
		}
	}

	return nil
}

// IPToInfos adds geolocation and ISP to an address.
type IPToInfos struct {
	BaseURL string
}

// Meta describes the transform.
func (IPToInfos) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_infos", Category: "Ip", Input: "ip", Output: "ip",
		Description: "Geolocate an IP address and name its ISP (ip-api.com)."}
}

// ipAPIAnswer is what ip-api.com answers.
type ipAPIAnswer struct {
	Status  string  `json:"status"`
	Message string  `json:"message"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Country string  `json:"country"`
	City    string  `json:"city"`
	ISP     string  `json:"isp"`
}

// Run enriches each address in place.
func (e IPToInfos) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")

		var a ipAPIAnswer
		if _, err := httpx.GetJSON(ctx, x, e.BaseURL+url.PathEscape(addr), nil, &a); err != nil {
			x.Warn("%s: %v", addr, err)

			continue
		}

		if a.Status != "success" {
			x.Warn("%s: ip-api: %s", addr, a.Message)

			continue
		}

		if _, err := x.Node(ctx, "ip", map[string]any{
			"address": addr, "latitude": a.Lat, "longitude": a.Lon, "country": a.Country, "city": a.City, "isp": a.ISP,
		}); err != nil {
			x.Warn("%s: %v", addr, err)

			continue
		}

		x.Found("geolocated %s to %s, %s (%s)", addr, a.City, a.Country, a.ISP)
	}

	return nil
}

// IPToASN links an address to the AS announcing it.
type IPToASN struct{ ripestat.Client }

// Meta describes the transform.
func (IPToASN) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_asn", Category: "Ip", Input: "ip", Output: "asn",
		Description: "Find the autonomous system announcing an IP address (RIPEstat)."}
}

// Run enriches each address.
func (e IPToASN) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.LinkASNs(ctx, x, in, props.Str(in.Properties, "address"), "BELONGS_TO")
	}

	return nil
}

// Port is one open port as a source reports it. Only the number is
// certain, the rest is what the source happens to know.
type Port struct {
	Number   int
	Protocol string
	Service  string
	Banner   string
}

// PortSource is where open ports of an address come from: InternetDB
// without touching the target, nmap by connecting to it.
type PortSource interface {
	Name() string
	OpenPorts(ctx context.Context, x *transform.Exec, ip string) ([]Port, error)
}

// InternetDB is the passive port source.
type InternetDB struct {
	BaseURL string
}

// Name labels the source in logs and on port nodes.
func (InternetDB) Name() string { return "shodan-internetdb" }

// OpenPorts reads what InternetDB knows about ip.
func (s InternetDB) OpenPorts(ctx context.Context, x *transform.Exec, ip string) ([]Port, error) {
	var answer struct {
		Ports []int `json:"ports"`
	}

	if _, err := httpx.GetJSON(ctx, x, s.BaseURL+url.PathEscape(ip), nil, &answer); err != nil {
		return nil, err
	}

	ports := make([]Port, 0, len(answer.Ports))
	for _, p := range answer.Ports {
		ports = append(ports, Port{Number: p, Protocol: "TCP"})
	}

	return ports, nil
}

// Nmap is the active port source, the nmap binary on the server.
type Nmap struct {
	Scanner nmap.Scanner
}

// Name labels the source in logs and on port nodes.
func (Nmap) Name() string { return "nmap" }

// OpenPorts scans the top_ports most common ports of ip.
func (s Nmap) OpenPorts(ctx context.Context, x *transform.Exec, ip string) ([]Port, error) {
	top, _ := strconv.Atoi(x.Param("top_ports"))
	scanner := s.Scanner
	set := x.Tools.Get(nmap.ToolName)
	scanner.AllowPrivate = set.AllowPrivate
	if scanner.Path == "" {
		scanner.Path = set.Path
	}

	found, err := scanner.Scan(ctx, ip, top)
	if err != nil {
		return nil, err
	}

	ports := make([]Port, 0, len(found))
	for _, p := range found {
		ports = append(ports, Port{Number: p.Number, Protocol: p.Protocol, Service: p.Service, Banner: p.Banner})
	}

	return ports, nil
}

// IPToPorts lists open ports of an address. The passive source is the default, the active one connects to the
// target and is chosen by name.
type IPToPorts struct {
	Passive PortSource
	Active  PortSource
}

// Meta describes the transform.
func (IPToPorts) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_ports", Category: "Ip", Input: "ip", Output: "port",
		Description: "List open TCP ports of an IP address, from Shodan InternetDB without contacting the target, or by scanning it with nmap on the server.",
		Params: []transform.Param{
			{Name: "source", Type: transform.ParamSelect, Default: "passive", Description: "Where the ports come from",
				Options: []transform.Option{{Label: "Passive (Shodan InternetDB, nothing sent to the target)", Value: "passive"},
					{Label: "Active (nmap on the server, connects to the target)", Value: "active"}}},
			{Name: "top_ports", Type: transform.ParamNumber, Default: strconv.Itoa(nmap.DefaultTopPorts),
				Description: "Active scan only: how many of the most common ports to try, at most " + strconv.Itoa(nmap.MaxTopPorts)},
		}}
}

// Run enriches each address. A server without nmap fails the active
// scan outright, since every address would fail the same way.
func (e IPToPorts) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	source := e.Passive
	if x.Param("source") == "active" {
		source = e.Active
	}

	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")
		ports, err := source.OpenPorts(ctx, x, addr)
		if errors.Is(err, nmap.ErrNotInstalled) {
			return err
		}

		if err != nil {
			x.Warn("%s: %s: %v", in.Label, source.Name(), err)

			continue
		}

		for _, p := range ports {
			port := map[string]any{"number": p.Number, "protocol": cmp.Or(p.Protocol, "TCP"), "state": "open"}
			if p.Service != "" {
				port["service"] = p.Service
			}

			if p.Banner != "" {
				port["banner"] = p.Banner
			}

			if _, err := x.NodeAndLink(ctx, in, "HAS_PORT", "port", port); err != nil {
				x.Warn("%s: port %d: %v", in.Label, p.Number, err)
			}
		}

		x.Info("%s: %d open ports known to %s", in.Label, len(ports), source.Name())
	}

	return nil
}

// IPToFraudScore adds Scamalytics' fraud score of an address.
type IPToFraudScore struct {
	// BaseURL replaces https://<host>.scamalytics.com in tests.
	BaseURL string
}

// Meta describes the transform.
func (IPToFraudScore) Meta() transform.Meta {
	return transform.Meta{Name: "ip_to_fraudscore", Category: "Ip", Input: "ip", Output: "riskprofile",
		Description: "Score how likely an IP address is used for fraud, and flag proxies and VPNs (Scamalytics).",
		Params: []transform.Param{
			{Name: "SCAMLYTICS_USERNAME", Type: transform.ParamSecret, Required: true, Description: "The Scamalytics username."},
			{Name: "SCAMLYTICS_API_KEY", Type: transform.ParamSecret, Required: true, Description: "The Scamalytics API key."},
			{Name: "SCAMLYTICS_API_HOST", Type: transform.ParamString, Default: scamalyticsHost,
				Description: "The Scamalytics API host prefix of your account, such as api12."},
		}}
}

// hostLabelRe is one DNS label, what an account's API host prefix is.
var hostLabelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Run enriches each address.
func (e IPToFraudScore) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	base := e.BaseURL
	if base == "" {
		host := x.Param("SCAMLYTICS_API_HOST")
		if !hostLabelRe.MatchString(host) {
			return fmt.Errorf("SCAMLYTICS_API_HOST %q is not a host prefix such as api12", host)
		}

		base = "https://" + host + ".scamalytics.com"
	}

	for _, in := range inputs {
		addr := props.Str(in.Properties, "address")
		q := url.Values{"key": {x.Param("SCAMLYTICS_API_KEY")}, "ip": {addr}}

		var answer struct {
			Scamalytics struct {
				Status string  `json:"status"`
				Error  string  `json:"error"`
				Score  float64 `json:"scamalytics_score"`
				Risk   string  `json:"scamalytics_risk"`
				Proxy  struct {
					Datacenter bool `json:"is_datacenter"`
					VPN        bool `json:"is_vpn"`
					ICloud     bool `json:"is_apple_icloud_private_relay"`
					AWS        bool `json:"is_amazon_aws"`
					Google     bool `json:"is_google"`
				} `json:"scamalytics_proxy"`
			} `json:"scamalytics"`
		}

		u := base + "/v3/" + url.PathEscape(x.Param("SCAMLYTICS_USERNAME")) + "?" + q.Encode()
		if _, err := httpx.GetJSON(ctx, x, u, nil, &answer); err != nil {
			x.Warn("%s: %v", addr, err)

			continue
		}

		s := answer.Scamalytics
		if s.Status != "ok" {
			x.Warn("%s: scamalytics: %s %s", addr, s.Status, s.Error)

			continue
		}

		var flags []any
		for _, f := range []struct {
			on    bool
			label string
		}{{s.Proxy.Datacenter, "Datacenter"}, {s.Proxy.VPN, "VPN"}, {s.Proxy.ICloud, "iCloud Private Relay"},
			{s.Proxy.AWS, "Amazon AWS"}, {s.Proxy.Google, "Google"}} {
			if f.on {
				flags = append(flags, f.label)
			}
		}

		profile := map[string]any{"entity_id": addr, "entity_type": "IP address", "overall_risk_score": s.Score,
			"risk_level": s.Risk, "risk_factors": flags, "source": "Scamalytics",
			"last_updated": time.Now().UTC().Format(time.RFC3339)}
		if _, err := x.NodeAndLink(ctx, in, "HAS_RISK_PROFILE", "riskprofile", profile); err != nil {
			x.Warn("%s: %v", addr, err)
		}
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		IPToAbuseIPDB{BaseURL: AbuseIPDBURL},
		IPToGreyNoise{BaseURL: GreyNoiseURL},
		IPToVirusTotal{BaseURL: VirusTotalURL},
		IPToCensys{BaseURL: CensysURL},
		IPToShodan{BaseURL: ShodanURL},
		IPToDomain{CrtshURL: crtsh.URL},
		IPToInfos{BaseURL: IPAPIURL},
		IPToASN{Client: ripestat.Public},
		IPToPorts{Passive: InternetDB{BaseURL: InternetDBURL}, Active: Nmap{}},
		IPToFraudScore{},
		IPToDehashed{Client: dehashed.Public},
	}
}
