// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"

	"golang.org/x/net/publicsuffix"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/subdomains"
)

// DomainToSubdomains links a domain to its subdomains: from subfinder's
// passive sources, falling back to certificate transparency logs when
// those find none.
type DomainToSubdomains struct {
	// BaseURL is crt.sh.
	BaseURL string

	// Passive enumerates subdomains, subfinder unless a test swaps it.
	Passive func(ctx context.Context, domain string) ([]string, error)
}

// Meta describes the transform.
func (DomainToSubdomains) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_subdomains", Category: "Domain", Input: "domain", Output: "domain",
		Description: "Find subdomains of a domain with subfinder's passive sources, or else in certificate transparency logs (crt.sh)."}
}

// Run enriches each domain.
func (e DomainToSubdomains) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	finder := subdomains.Finder{CrtURL: e.BaseURL, Passive: e.Passive}
	for _, in := range inputs {
		domain := props.Str(in.Properties, "domain")
		found, source := finder.Find(ctx, x, domain)
		for _, sub := range found {
			if _, err := x.NodeAndLink(ctx, in, "HAS_SUBDOMAIN", "domain", map[string]any{"domain": sub}); err != nil {
				x.Warn("%s: %v", sub, err)
			}
		}

		x.Info("%s: %d subdomains found with %s", domain, len(found), source)
	}

	return nil
}

// DomainToRootDomain links a subdomain's registrable domain to it.
type DomainToRootDomain struct{}

// Meta describes the transform.
func (DomainToRootDomain) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_root_domain", Category: "Domain", Input: "domain", Output: "domain",
		Description: "Find the registrable root domain of a subdomain."}
}

// Run enriches each domain.
//
// The root is cut by the public suffix list: a hand list of country
// suffixes gets every other multi-label suffix wrong.
func (DomainToRootDomain) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		name := props.Str(in.Properties, "domain")
		root, err := publicsuffix.EffectiveTLDPlusOne(name)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if root == name {
			continue
		}

		rootNode, err := x.Node(ctx, "domain", map[string]any{"domain": root})
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if err := x.Link(ctx, rootNode, in, "HAS_SUBDOMAIN"); err != nil {
			return err
		}

		x.Found("%s -> HAS_SUBDOMAIN -> %s", root, name)
	}

	return nil
}
