// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// DomainToDNS reads a domain's records. Addresses become ip nodes, the
// CNAME, MX and NS targets domain nodes, TXT records dnsrecord nodes.
// domain_to_ip is the plain lookup for when only the addresses matter.
type DomainToDNS struct{}

// Meta describes the transform.
func (DomainToDNS) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_dns", Category: "Domain", Input: "domain", Output: "ip",
		Description: "Read a domain's DNS records: addresses, CNAME, MX and NS hosts as domains, and TXT records.",
		Params: []transform.Param{{Name: "ipv6", Type: transform.ParamSelect, Default: "true", Options: transform.OnOff,
			Description: "Also resolve AAAA (IPv6) records"}}}
}

// Run enriches each domain. A record kind a domain has not got is
// simply absent, only a domain with nothing at all is reported.
func (DomainToDNS) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	network := "ip4"
	if x.Param("ipv6") == "true" {
		network = "ip"
	}

	for _, in := range inputs {
		name := props.Str(in.Properties, "domain")
		found := 0
		link := func(label, typ string, nodeProps, edge map[string]any) {
			n, err := x.Node(ctx, typ, nodeProps)
			if err == nil {
				err = x.LinkWith(ctx, in, n, label, edge)
			}

			if err != nil {
				x.Warn("%s: %v", in.Label, err)

				return
			}

			found++
			x.Found("%s -> %s -> %s", in.Label, label, n.Label)
		}

		if cname, err := x.Resolver.LookupCNAME(ctx, name); err == nil {
			if c := hostname(cname); c != "" && c != strings.ToLower(name) {
				link("ALIAS_OF", "domain", map[string]any{"domain": c}, nil)
			}
		}

		addrs, addrErr := x.Resolver.LookupNetIP(ctx, network, name)
		for _, a := range addrs {
			link("RESOLVES_TO", "ip", map[string]any{"address": a.Unmap().String()}, nil)
		}

		if mxs, err := x.Resolver.LookupMX(ctx, name); err == nil {
			for _, mx := range mxs {
				if h := hostname(mx.Host); h != "" {
					link("HAS_MX", "domain", map[string]any{"domain": h}, map[string]any{"priority": int(mx.Pref)})
				}
			}
		}

		if nss, err := x.Resolver.LookupNS(ctx, name); err == nil {
			for _, ns := range nss {
				if h := hostname(ns.Host); h != "" {
					link("HAS_NS", "domain", map[string]any{"domain": h}, nil)
				}
			}
		}

		if txts, err := x.Resolver.LookupTXT(ctx, name); err == nil {
			for _, txt := range txts {
				if txt = strings.TrimSpace(txt); txt != "" {
					link("HAS_DNS_RECORD", "dnsrecord", map[string]any{"value": txt, "record_type": "TXT", "source": "dns"}, nil)
				}
			}
		}

		if found == 0 {
			x.Warn("%s: no DNS records: %v", in.Label, addrErr)

			continue
		}

		x.Info("%s: %d DNS records", in.Label, found)
	}

	return nil
}

// hostname is a DNS answer as a domain name: lowercase, no final dot.
func hostname(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
}

// DomainToIP links a domain to the addresses it resolves to.
type DomainToIP struct{}

// Meta describes the transform.
func (DomainToIP) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_ip", Category: "Domain", Input: "domain", Output: "ip",
		Description: "Resolve a domain to its IPv4 and IPv6 addresses."}
}

// Run enriches each domain with every address it resolves to.
func (DomainToIP) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		addrs, err := x.Resolver.LookupIPAddr(ctx, props.Str(in.Properties, "domain"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, a := range addrs {
			if _, err := x.NodeAndLink(ctx, in, "RESOLVES_TO", "ip", map[string]any{"address": a.IP.String()}); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}
	}

	return nil
}
