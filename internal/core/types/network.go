// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

var (
	asnPrefixRe    = regexp.MustCompile(`(?i)^AS`)
	asnDetectRe    = regexp.MustCompile(`(?i)^AS([0-9]+)$`)
	domainCheckRe  = regexp.MustCompile(`^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	domainDetectRe = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$`)
	websiteRe      = regexp.MustCompile(`^https?://[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}(/.*)?$`)
	pyIntRe        = regexp.MustCompile(`^[+-]?[0-9]+(?:_[0-9]+)*$`)
)

// maxASN is the largest 32 bit AS number.
const maxASN = 4294967295

// ccSecondLevel are the suffixes under which a root domain has three
// labels.
var ccSecondLevel = []string{
	".co.uk", ".com.au", ".org.uk", ".net.uk", ".gov.uk", ".ac.uk",
	".co.nz", ".com.sg", ".co.jp", ".co.kr", ".com.br", ".com.mx",
}

// pyInt parses an integer as Python's int() does for a decimal string:
// surrounding whitespace, a sign and single underscores between digits.
func pyInt(s string) (int64, bool) {
	s = pyStrip(s)
	if !pyIntRe.MatchString(s) {
		return 0, false
	}

	n, err := strconv.ParseInt(strings.ReplaceAll(s, "_", ""), 10, 64)

	return n, err == nil
}

// isRootDomain reports whether a domain is registrable as it is, such
// as example.com or bbc.co.uk.
func isRootDomain(domain string) bool {
	if strings.Contains(domain, "://") {
		if h := urlHostname(domain); h != "" {
			domain = h
		}
	}

	parts := strings.Split(domain, ".")
	for _, cc := range ccSecondLevel {
		if strings.HasSuffix(domain, cc) {
			return len(parts) == 3
		}
	}

	return len(parts) == 2
}

// normalizeDomain reduces a domain or URL to its lowercase hostname.
// The input is not trimmed.
func normalizeDomain(v string) (string, error) {
	candidate := v
	if !strings.Contains(v, "://") {
		candidate = "http://" + v
	}

	host := urlHostname(candidate)
	if host == "" {
		host = v
	}

	if !strings.Contains(host, ".") || !domainCheckRe.MatchString(host) {
		return "", fmt.Errorf("invalid domain: %s", v)
	}

	return host, nil
}

func networkTypes() []spec {
	return []spec{
		{
			name: "asn", title: "ASN", category: "network", icon: "Network", color: "#D97474",
			after: func(p map[string]any) error {
				raw := pyStrip(str(p, "asn_str"))
				n, ok := pyInt(asnPrefixRe.ReplaceAllString(raw, ""))
				if !ok {
					return fieldErr("asn", "asn_str", "invalid ASN format: %s", raw)
				}

				if n < 0 || n > maxASN {
					return fieldErr("asn", "asn_str", "ASN number must be between 0 and %d, got %d", int64(maxASN), n)
				}

				p["asn_str"] = "AS" + strconv.FormatInt(n, 10)
				if p["number"] == nil {
					p["number"] = n
				}

				return nil
			},
			label: func(p map[string]any) string {
				if truthy(p["name"]) {
					return str(p, "asn_str") + " - " + pyStr(p["name"])
				}

				return str(p, "asn_str")
			},
			fromString: into("asn_str"),
			detect: func(s string) bool {
				m := asnDetectRe.FindStringSubmatch(s)
				if m == nil {
					return false
				}

				n, err := strconv.ParseUint(m[1], 10, 64)

				return err == nil && n >= 1 && n <= maxASN
			},
		},
		{
			// Detection and validation agree here: host bits are masked
			// off, so 8.8.8.8/24 is detected and kept as 8.8.8.0/24.
			name: "cidr", title: "CIDR", category: "network", icon: "Network", color: "#80BF80",
			after: func(p map[string]any) error {
				n, err := parseCIDR(pyStrip(str(p, "network")))
				if err != nil {
					return fieldErr("cidr", "network", "not a valid network: %s", str(p, "network"))
				}

				p["network"] = n.String()

				return nil
			},
			label:      field("network"),
			fromString: into("network"),
			detect: func(s string) bool {
				if !strings.Contains(s, "/") {
					return false
				}

				_, err := parseCIDR(s)

				return err == nil
			},
		},
		{
			name: "domain", title: "Domain", category: "network", icon: "Globe", color: "#66A892",
			after: func(p map[string]any) error {
				host, err := normalizeDomain(str(p, "domain"))
				if err != nil {
					return fieldErr("domain", "domain", "%s", err)
				}

				p["domain"] = host
				p["root"] = isRootDomain(host)

				return nil
			},
			label:      field("domain"),
			fromString: into("domain"),
			detect: func(s string) bool {
				if len(s) > 253 || !domainDetectRe.MatchString(s) {
					return false
				}

				return !strings.Contains(s, "..") && !strings.HasPrefix(s, ".") && !strings.HasSuffix(s, ".")
			},
		},
		{
			name: "ip", title: "IP", category: "network", icon: "Network", color: "#5AA1C8",
			after: func(p map[string]any) error {
				if _, err := netip.ParseAddr(str(p, "address")); err != nil {
					return fieldErr("ip", "address", "invalid IP address: %s", str(p, "address"))
				}

				return nil
			},
			label:      field("address"),
			fromString: into("address"),
			detect: func(s string) bool {
				_, err := netip.ParseAddr(s)

				return err == nil
			},
		},
		{
			name: "port", title: "Port", category: "network", icon: "Plug", color: "#4CB5DA",
			after: func(p map[string]any) error {
				n, _ := p["number"].(int64)
				if n < 0 || n > 65535 {
					return fieldErr("port", "number", "port number must be between 0 and 65535, got %d", n)
				}

				return nil
			},
			label: func(p map[string]any) string {
				parts := []string{pyStr(p["number"])}
				if truthy(p["service"]) {
					parts = append(parts, pyStr(p["service"]))
				}

				if truthy(p["protocol"]) {
					parts = append(parts, "("+pyStr(p["protocol"])+")")
				}

				return strings.Join(parts, " ")
			},
			fromString: func(s string) (map[string]any, error) {
				n, ok := pyInt(s)
				if !ok {
					return nil, fieldErr("port", "number", "not a port number: %s", s)
				}

				return map[string]any{"number": n}, nil
			},
			detect: func(s string) bool {
				if !allDigits(s) {
					return false
				}

				n, err := strconv.Atoi(s)

				return err == nil && n <= 65535
			},
		},
		{
			name: "website", title: "Website", category: "network", icon: "Globe", color: "#D279A6",
			label:      field("url"),
			fromString: into("url"),
			detect:     websiteRe.MatchString,
		},
		{
			name: "dnsrecord", title: "DNS Record", category: "network", icon: "Database", color: "#BFAF80",
			label: field("value"),
		},
		{
			name: "sslcertificate", title: "SSL Certificate", category: "network", icon: "Lock", color: "#BFAF80",
			label:      field("subject"),
			fromString: into("subject"),
		},
		{
			name: "webtracker", title: "Web Tracker", category: "network", icon: "Target", color: "#C7BF50",
			label: func(p map[string]any) string {
				if truthy(p["name"]) {
					return pyStr(p["name"]) + "-" + str(p, "tracker_id")
				}

				return str(p, "tracker_id")
			},
			fromString: into("tracker_id"),
		},
		{
			name: "technology", title: "Technology", category: "network",
			label: func(p map[string]any) string {
				if truthy(p["version"]) {
					return str(p, "name") + " " + pyStr(p["version"])
				}

				return str(p, "name")
			},
			fromString: func(s string) (map[string]any, error) {
				s = pyStrip(s)
				name, version, found := strings.Cut(s, ":")
				if !found {
					return map[string]any{"name": s}, nil
				}

				props := map[string]any{"name": pyStrip(name)}
				if v := pyStrip(version); v != "" {
					props["version"] = v
				}

				return props, nil
			},
		},
		{
			name: "whois", title: "Whois", category: "network", icon: "Info", color: "#9B6F9B",
			label: func(p map[string]any) string {
				domain, _ := p["domain"].(map[string]any)
				label := str(domain, "domain")
				if org, ok := p["organization"].(map[string]any); ok && truthy(org) {
					label += " - " + pyStr(org["name"])
				}

				return label
			},
		},
	}
}
