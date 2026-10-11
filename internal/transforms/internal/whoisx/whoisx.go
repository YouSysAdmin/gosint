// Gosint, Copyright (c) 2026 YouSysAdmin

// Package whoisx writes what WHOIS contacts reveal, whichever source
// they came from: the person, their email, phone and address, and
// their company, each linked to the domain.
package whoisx

import (
	"context"
	"net/mail"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Redacted reports whether a WHOIS value is empty or privacy-masked.
func Redacted(v string) bool {
	u := strings.ToUpper(strings.TrimSpace(v))

	return u == "" || strings.Contains(u, "REDACTED") || strings.Contains(u, "PRIVACY") ||
		strings.Contains(u, "ANONYMISED") || strings.Contains(u, "NOT DISCLOSED")
}

// Email reads a WHOIS email field. Registrars hiding the contact often
// put a contact form URL there instead, returned as contactURL.
func Email(v string) (email, contactURL string) {
	v = strings.TrimSpace(v)
	if Redacted(v) {
		return "", ""
	}

	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return "", v
	}

	if a, err := mail.ParseAddress(v); err == nil {
		return a.Address, ""
	}

	return "", ""
}

// Contact is one WHOIS contact.
type Contact struct {
	Kind                        string
	Name, Company, Email, Phone string
	Address, City, Zip, Country string
}

// Link writes what a contact reveals about domain.
func Link(ctx context.Context, x *transform.Exec, domain graphmodel.Node, c Contact) {
	var person *graphmodel.Node
	if !Redacted(c.Name) {
		first, last, _ := strings.Cut(strings.TrimSpace(c.Name), " ")
		n, err := x.Node(ctx, "individual", map[string]any{"full_name": c.Name, "first_name": first, "last_name": last})
		if err == nil {
			person = &n
			if err := x.Link(ctx, n, domain, "IS_"+strings.ToUpper(c.Kind)+"_CONTACT"); err == nil {
				x.Found("%s is the %s contact of %s", c.Name, c.Kind, domain.Label)
			}
		}
	}

	if !Redacted(c.Company) {
		org, err := x.Node(ctx, "organization", map[string]any{"name": c.Company})
		if err == nil {
			_ = x.Link(ctx, org, domain, "HAS_DOMAIN")
			if person != nil {
				_ = x.Link(ctx, *person, org, "WORKS_FOR")
			}
		}
	}

	if person == nil {
		return
	}

	for e := range strings.SplitSeq(c.Email, ",") {
		if e, _ = Email(e); e != "" {
			if _, err := x.NodeAndLink(ctx, *person, "HAS_EMAIL", "email", map[string]any{"email": e}); err != nil {
				x.Warn("%s: %v", c.Name, err)
			}
		}
	}

	if !Redacted(c.Phone) {
		if _, err := x.NodeAndLink(ctx, *person, "HAS_PHONE", "phone", map[string]any{"number": c.Phone}); err != nil {
			x.Warn("%s: %v", c.Name, err)
		}
	}

	if !Redacted(c.Address) && !Redacted(c.City) && !Redacted(c.Zip) && !Redacted(c.Country) {
		loc := map[string]any{"address": c.Address, "city": c.City, "zip": c.Zip, "country": c.Country}
		if _, err := x.NodeAndLink(ctx, *person, "LIVES_AT", "location", loc); err != nil {
			x.Warn("%s: %v", c.Name, err)
		}
	}
}
