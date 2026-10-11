// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/page"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// DomainToWebsite finds the website a domain serves, over https first
// and then http.
type DomainToWebsite struct{}

// Meta describes the transform.
func (DomainToWebsite) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_website", Category: "Domain", Input: "domain", Output: "website",
		Description: "Fetch the website a domain serves, with its title, description, status, server and redirects."}
}

// Run enriches each domain.
func (DomainToWebsite) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		domain := props.Str(in.Properties, "domain")
		var (
			got  *httpx.Fetched
			last string
		)

		for _, scheme := range []string{"https://", "http://"} {
			f, err := httpx.Fetch(ctx, x, scheme+domain)
			if err == nil && f.Status < 400 {
				got = f

				break
			}

			if err != nil {
				last = err.Error()
			} else {
				last = fmt.Sprintf("%s answered %d", scheme+domain, f.Status)
			}
		}

		if got == nil {
			x.Warn("%s: no website answered: %s", in.Label, last)

			continue
		}

		p := page.Parse(got.Body)
		headers := map[string]any{}
		for _, h := range []string{"Content-Type", "Server", "X-Powered-By"} {
			if v := got.Header.Get(h); v != "" {
				headers[strings.ToLower(h)] = v
			}
		}

		redirects := make([]any, 0, len(got.Redirects))
		for _, r := range got.Redirects {
			redirects = append(redirects, r)
		}

		// The page text is website_to_text's, it is too long for a property.
		site := map[string]any{
			"url": got.URL, "domain": map[string]any{"domain": domain}, "active": true,
			"status_code": got.Status, "headers": headers, "redirects": redirects,
			"title": p.Title, "description": p.Description,
		}
		if got.ReachedURL != "" && got.ReachedURL != got.URL {
			site["final_url"] = got.ReachedURL
		}

		if _, err := x.NodeAndLink(ctx, in, "HAS_WEBSITE", "website", site); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// DomainToTLS reads the certificate a domain serves over https and
// records the certificate itself.
type DomainToTLS struct{}

// Meta describes the transform.
func (DomainToTLS) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_tls", Category: "Domain", Input: "domain", Output: "sslcertificate",
		Description: "Read the TLS certificate a domain serves over https."}
}

// Run enriches each domain.
func (DomainToTLS) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		got, err := httpx.Fetch(ctx, x, "https://"+props.Str(in.Properties, "domain"))
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if got.TLS == nil || len(got.TLS.PeerCertificates) == 0 {
			x.Warn("%s: no certificate", in.Label)

			continue
		}

		if _, err := x.NodeAndLink(ctx, in, "TLS_INFO", "sslcertificate", certProps(got.TLS.PeerCertificates[0])); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// certProps are a certificate's sslcertificate properties.
func certProps(c *x509.Certificate) map[string]any {
	sans := make([]any, 0, len(c.DNSNames))
	wildcard := false
	for _, n := range c.DNSNames {
		sans = append(sans, n)
		wildcard = wildcard || strings.HasPrefix(n, "*.")
	}

	now := time.Now()

	return map[string]any{
		"subject":             c.Subject.String(),
		"issuer":              c.Issuer.String(),
		"serial_number":       c.SerialNumber.Text(16),
		"valid_from":          c.NotBefore.UTC().Format(time.RFC3339),
		"valid_until":         c.NotAfter.UTC().Format(time.RFC3339),
		"signature_algorithm": c.SignatureAlgorithm.String(),
		"key_type":            c.PublicKeyAlgorithm.String(),
		"san_domains":         sans,
		"is_expired":          now.After(c.NotAfter),
		"is_valid":            now.After(c.NotBefore) && now.Before(c.NotAfter),
		"is_self_signed":      c.Subject.String() == c.Issuer.String(),
		"is_wildcard":         wildcard,
		"source":              "tls",
	}
}
