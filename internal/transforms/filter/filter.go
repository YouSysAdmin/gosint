// Gosint, Copyright (c) 2026 YouSysAdmin

// Package filter holds the transforms that NARROW a set instead of
// growing it: the IPs behind a CDN dropped before a port probe, only the
// root domains kept, only the domains that resolve. Each takes and
// gives the same type, so it sits anywhere in a flow. A kept entity is
// written again (which is what the next step receives), a dropped one
// is only marked, so it stays in the sketch but travels no further.
package filter

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/cdnranges"
)

// The flag colours a dropped entity gets, from the console's set.
const (
	flagDropped = "yellow"
	flagDead    = "red"
)

// RangeSource gives a provider's address ranges. cdnranges.Table is the
// one shipped, a test fakes it.
type RangeSource interface {
	Names() []string
	Prefixes(ctx context.Context, x *transform.Exec, provider string) ([]netip.Prefix, error)
}

var modeParam = transform.Param{Name: "mode", Type: transform.ParamSelect, Default: "drop",
	Description: "drop passes on what is NOT matched, keep passes on only what is",
	Options:     []transform.Option{{Label: "Drop the matched", Value: "drop"}, {Label: "Keep only the matched", Value: "keep"}}}

// IPDropCDN drops the addresses that belong to a CDN or a cloud front:
// they say nothing about the host behind them.
type IPDropCDN struct {
	Ranges RangeSource
}

// Meta describes the transform.
func (e IPDropCDN) Meta() transform.Meta {
	return transform.Meta{Name: "ip_drop_cdn", Category: "Filter", Input: "ip", Output: "ip",
		Description: "Narrows: drop the IP addresses that belong to a CDN or cloud front (Cloudflare, Fastly, AWS, Google Cloud), " +
			"so the next step probes only the hosts themselves.",
		Params: []transform.Param{
			{Name: "providers", Type: transform.ParamString, Default: strings.Join(e.Ranges.Names(), ","),
				Description: "Which providers count, comma-separated: " + strings.Join(e.Ranges.Names(), ", ")},
			modeParam,
		}}
}

// Run marks each matched address with the provider and passes the rest.
func (e IPDropCDN) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	var sets []struct {
		name     string
		prefixes []netip.Prefix
	}

	for name := range strings.SplitSeq(x.Param("providers"), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		prefixes, err := e.Ranges.Prefixes(ctx, x, name)
		if err != nil {
			return err
		}

		sets = append(sets, struct {
			name     string
			prefixes []netip.Prefix
		}{name, prefixes})
	}

	keepMatched := x.Param("mode") == "keep"
	kept := 0
	for _, in := range inputs {
		addr, err := netip.ParseAddr(props.Str(in.Properties, "address"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		provider := ""
		for _, s := range sets {
			if cdnranges.Contains(s.prefixes, addr) {
				provider = s.name

				break
			}
		}

		matched := provider != ""
		if matched {
			if err := x.Annotate(ctx, in, map[string]any{"cdn": provider}, flagDropped); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}

		if matched == keepMatched {
			x.Pass(in)
			kept++
		}
	}

	x.Info("kept %d of %d addresses", kept, len(inputs))

	return nil
}

// IPPublicOnly drops private, loopback, link-local and other addresses
// that are nobody's on the internet.
type IPPublicOnly struct{}

// Meta describes the transform.
func (IPPublicOnly) Meta() transform.Meta {
	return transform.Meta{Name: "ip_public_only", Category: "Filter", Input: "ip", Output: "ip",
		Description: "Narrows: drop private, loopback, link-local and reserved addresses, keep the ones routable on the internet. A lab with worker.allow_private_targets on does not want this step."}
}

// Run passes the public addresses, marks the rest.
func (IPPublicOnly) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	kept := 0
	for _, in := range inputs {
		addr, err := netip.ParseAddr(props.Str(in.Properties, "address"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !transform.Public(addr) {
			if err := x.Annotate(ctx, in, map[string]any{"scope": "private"}, flagDropped); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}

			continue
		}

		x.Pass(in)
		kept++
	}

	x.Info("kept %d of %d addresses", kept, len(inputs))

	return nil
}

// DomainRootOnly keeps the registrable domains, or only the subdomains.
type DomainRootOnly struct{}

// Meta describes the transform.
func (DomainRootOnly) Meta() transform.Meta {
	return transform.Meta{Name: "domain_root_only", Category: "Filter", Input: "domain", Output: "domain",
		Description: "Narrows: keep only the registrable root domains (example.com, not www.example.com), or only the subdomains.",
		Params: []transform.Param{{Name: "mode", Type: transform.ParamSelect, Default: "root",
			Description: "root keeps the registrable domains, sub keeps the subdomains",
			Options:     []transform.Option{{Label: "Root domains", Value: "root"}, {Label: "Subdomains", Value: "sub"}}}}}
}

// Run passes the domains of the chosen kind.
func (DomainRootOnly) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	wantRoot := x.Param("mode") != "sub"
	kept := 0
	for _, in := range inputs {
		name := props.Str(in.Properties, "domain")
		root, err := publicsuffix.EffectiveTLDPlusOne(name)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if (root == name) == wantRoot {
			x.Pass(in)
			kept++
		}
	}

	x.Info("kept %d of %d domains", kept, len(inputs))

	return nil
}

// DomainAlive keeps the domains that answer: a DNS record, or a web
// server over https or http. Every domain gets both checks, the result
// is logged with its reason and written onto it (alive, checked_at,
// http_status, check_error), and the flag says it at a glance: red when
// nothing answered, yellow when the name resolves but the site answers
// anything but a 200 (or nothing at all), no flag for a 200. The check
// param only picks what passes on to the next step.
type DomainAlive struct{}

// Meta describes the transform.
func (DomainAlive) Meta() transform.Meta {
	return transform.Meta{Name: "domain_alive", Category: "Filter", Input: "domain", Output: "domain",
		Description: "Narrows: keep only the domains that resolve, or that serve a website, and flag the dead ones red and the ones not answering 200 yellow.",
		Params: []transform.Param{
			{Name: "check", Type: transform.ParamSelect, Default: "dns",
				Description: "dns keeps what resolves to an address, http keeps what answers over https or http",
				Options:     []transform.Option{{Label: "Resolves", Value: "dns"}, {Label: "Serves a website", Value: "http"}}},
			{Name: "with_subdomains", Type: transform.ParamBool, Default: "false",
				Description: "Also check every subdomain found under the selected ones, all the way down"},
		}}
}

// probes bounds how many domains are checked at once, maxDomains how
// many a run takes with its subdomains.
const (
	probes     = 8
	maxDomains = 2000
)

// verdict is what the checks found out about one domain.
type verdict struct {
	addrs  []string
	dnsErr string
	status int
	scheme string
	webErr string
}

func (v verdict) resolves() bool { return len(v.addrs) > 0 }

func (v verdict) answers() bool { return v.status > 0 }

// alive is whether anything answered, an address or a web server.
func (v verdict) alive() bool { return v.resolves() || v.answers() }

// flag is the colour the verdict earns: red when nothing answered,
// yellow when the site answered anything but a 200 or did not answer
// while the name resolves, none for a 200.
func (v verdict) flag() string {
	switch {
	case !v.alive():
		return flagDead
	case v.status != http.StatusOK:
		return flagDropped
	default:
		return ""
	}
}

// errorText is what went wrong, for the check_error field: the DNS
// error, and the web errors when no scheme answered.
func (v verdict) errorText() string {
	var parts []string
	if v.dnsErr != "" {
		parts = append(parts, "DNS: "+v.dnsErr)
	}

	if v.webErr != "" {
		parts = append(parts, v.webErr)
	}

	return strings.Join(parts, ", ")
}

// reason reads for the log: the addresses or the DNS error, then the
// status or the web errors.
func (v verdict) reason() string {
	var parts []string
	if v.resolves() {
		parts = append(parts, "resolves to "+strings.Join(v.addrs, ", "))
	} else {
		parts = append(parts, "DNS: "+v.dnsErr)
	}

	if v.answers() {
		parts = append(parts, fmt.Sprintf("%s answers %d", v.scheme, v.status))
	} else {
		parts = append(parts, "no web server ("+v.webErr+")")
	}

	return strings.Join(parts, ", ")
}

// Run checks every domain a few at a time, writes each verdict onto it
// and passes the ones the check param keeps.
func (d DomainAlive) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	if x.Param("with_subdomains") == "true" {
		all, err := d.under(ctx, x, inputs)
		if err != nil {
			return err
		}

		if extra := len(all) - len(inputs); extra > 0 {
			x.Info("%d subdomains under the selected domains are checked too", extra)
		}

		inputs = all
	}

	found := make([]verdict, len(inputs))
	slots := make(chan struct{}, probes)

	var wg sync.WaitGroup
	for i, in := range inputs {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			found[i] = d.check(ctx, x, props.Str(in.Properties, "domain"))
		})
	}

	wg.Wait()

	now := time.Now().UTC().Format(time.RFC3339)
	web := x.Param("check") == "http"
	alive, kept := 0, 0
	for i, in := range inputs {
		v := found[i]
		p := map[string]any{"alive": v.alive(), "checked_at": now, "http_status": nil, "check_error": nil}
		if v.answers() {
			p["http_status"] = v.status
		}

		if e := v.errorText(); e != "" {
			p["check_error"] = e
		}

		if v.alive() {
			alive++
			x.Info("%s is alive: %s", in.Label, v.reason())
		} else {
			x.Warn("%s is dead: %s", in.Label, v.reason())
		}

		keep := v.resolves()
		if web {
			keep = v.answers()
		}

		if keep {
			merged := maps.Clone(in.Properties)
			maps.Copy(merged, p)
			if _, err := x.Node(ctx, in.Type, merged); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}

			kept++
		}

		if err := d.mark(ctx, x, in, p, v.flag()); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	x.Info("%d of %d domains are alive, %d pass on", alive, len(inputs), kept)

	return nil
}

// mark writes the verdict's fields and flag onto the domain. A verdict
// earning no flag clears the one an earlier run left.
func (DomainAlive) mark(ctx context.Context, x *transform.Exec, in graphmodel.Node, p map[string]any, flag string) error {
	if flag != "" {
		return x.Annotate(ctx, in, p, flag)
	}

	if err := x.Annotate(ctx, in, p, ""); err != nil {
		return err
	}

	for _, old := range []string{flagDead, flagDropped} {
		if err := x.Unflag(ctx, in, old); err != nil {
			return err
		}
	}

	return nil
}

// check resolves one domain and probes it over https then http.
func (DomainAlive) check(ctx context.Context, x *transform.Exec, name string) verdict {
	var v verdict
	addrs, err := x.Resolver.LookupNetIP(ctx, "ip", name)
	switch {
	case err != nil:
		v.dnsErr = err.Error()
	case len(addrs) == 0:
		v.dnsErr = "no address"
	default:
		for _, a := range addrs[:min(3, len(addrs))] {
			v.addrs = append(v.addrs, a.String())
		}
	}

	// Any answer is a web server, a 403 or a 404 included, the status
	// tells them apart.
	var reasons []string
	for _, scheme := range []string{"https://", "http://"} {
		status, err := httpx.Probe(ctx, x, scheme+name)
		if err == nil {
			v.status, v.scheme = status, scheme+name

			return v
		}

		reasons = append(reasons, scheme+name+": "+err.Error())
	}

	v.webErr = strings.Join(reasons, ", ")

	return v
}

// under is the inputs and every domain reached from them along outgoing
// relationships through domains, each once, at most maxDomains.
func (DomainAlive) under(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) ([]graphmodel.Node, error) {
	seen := map[string]bool{}
	out := make([]graphmodel.Node, 0, len(inputs))
	queue := slices.Clone(inputs)
	for _, n := range inputs {
		seen[n.ID] = true
	}

	for len(queue) > 0 && len(out) < maxDomains {
		n := queue[0]
		queue = queue[1:]
		out = append(out, n)

		nodes, edges, err := x.Neighbours(ctx, n)
		if err != nil {
			return nil, err
		}

		byID := map[string]graphmodel.Node{}
		for _, m := range nodes {
			byID[m.ID] = m
		}

		for _, e := range edges {
			child, ok := byID[e.TargetID]
			// A domain found under this one is named under it: an MX or NS
			// target is a neighbour too, but not a subdomain.
			if e.SourceID != n.ID || !ok || child.Type != "domain" || seen[child.ID] ||
				!strings.HasSuffix(props.Str(child.Properties, "domain"), "."+props.Str(n.Properties, "domain")) {
				continue
			}

			seen[child.ID] = true
			queue = append(queue, child)
		}
	}

	if len(queue) > 0 {
		x.Warn("stopped at %d domains, the rest are not checked", maxDomains)
	}

	return out, nil
}

// All is the package's transforms, pointed at the published ranges.
func All() []transform.Transform {
	return []transform.Transform{IPDropCDN{Ranges: cdnranges.Public}, IPPublicOnly{}, DomainRootOnly{}, DomainAlive{}}
}
