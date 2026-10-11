// Gosint, Copyright (c) 2026 YouSysAdmin

// Package subdomains finds the subdomains of a domain from free
// sources: subfinder's passive sources, and certificate transparency
// logs (crt.sh) when those find none. Shared by the domain and the
// website transforms.
package subdomains

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/crtsh"
)

// Finder is the lookup. A test points CrtURL at an httptest server and
// swaps Passive, nil Passive skips straight to crt.sh.
type Finder struct {
	// CrtURL is crt.sh.
	CrtURL string

	// Passive enumerates subdomains, subfinder in production.
	Passive func(ctx context.Context, domain string) ([]string, error)
}

// Find answers the subdomains of domain, sorted, and the source that
// found them. A source failing is logged on x, nothing found is an
// empty list.
func (f Finder) Find(ctx context.Context, x *transform.Exec, domain string) ([]string, string) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	seen := map[string]bool{}
	add := func(sub string) {
		sub = strings.ToLower(strings.TrimSpace(sub))

		// A suffix match on ".domain": a match on "domain" alone would
		// take evilexample.com for example.com.
		if !strings.Contains(sub, "*") && strings.HasSuffix(sub, "."+domain) {
			seen[sub] = true
		}
	}

	source := "subfinder"
	if f.Passive != nil {
		found, err := f.Passive(ctx, domain)
		if err != nil {
			x.Warn("%s: subfinder: %v", domain, err)
		}

		for _, sub := range found {
			add(sub)
		}
	}

	if len(seen) == 0 {
		if f.Passive != nil {
			x.Info("%s: subfinder found nothing, trying crt.sh", domain)
		}

		source = "crt.sh"
		entries, err := crtsh.Query(ctx, x, f.CrtURL, "%."+domain)
		if err != nil {
			x.Warn("%s: crt.sh: %v", domain, err)

			return nil, source
		}

		for _, entry := range entries {
			for sub := range strings.SplitSeq(entry.NameValue, "\n") {
				add(sub)
			}
		}
	}

	return slices.Sorted(maps.Keys(seen)), source
}
