// Gosint, Copyright (c) 2026 YouSysAdmin

package domain

import (
	"cmp"
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/whoxy"
	"github.com/yousysadmin/gosint/internal/transforms/internal/whoisx"
)

// WhoisXMLURL is the WhoisXML history API.
const WhoisXMLURL = "https://whois-history.whoisxmlapi.com/api/v1"

// DomainToWhois reads a domain's current WHOIS record. It replaces
// python-whois with likexian's client and parser.
type DomainToWhois struct{}

// Meta describes the transform.
func (DomainToWhois) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_whois", Category: "Domain", Input: "domain", Output: "whois",
		Description: "Read the current WHOIS record of a domain: registrar, dates, registrant."}
}

// Run enriches each domain.
func (DomainToWhois) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	// The dialer ends a query when the run is cancelled, and a referral
	// to another WHOIS server is held to public addresses like any target.
	client := whois.NewClient()
	client.SetDialer(ctxDialer{ctx: ctx, d: transform.TargetDialer(x.AllowPrivate)})
	client.SetTimeout(30 * time.Second)

	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}

		name := props.Str(in.Properties, "domain")
		raw, err := client.Whois(name)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		info, err := whoisparser.Parse(raw)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		record := map[string]any{"domain": map[string]any{"domain": name}}
		if d := info.Domain; d != nil {
			record["registry_domain_id"] = d.ID
			record["creation_date"] = d.CreatedDate
			record["expiration_date"] = d.ExpirationDate
		}

		if r := info.Registrar; r != nil {
			record["registrar"] = r.Name
		}

		email := ""
		if r := info.Registrant; r != nil {
			record["city"], record["country"] = r.City, r.Country
			if !whoisx.Redacted(r.Organization) {
				record["organization"] = map[string]any{"name": r.Organization}
			}

			var contactURL string
			email, contactURL = whoisx.Email(r.Email)
			if email != "" {
				record["email"] = map[string]any{"email": email}
			}

			if contactURL != "" {
				record["contact_url"] = contactURL
			}
		}

		w, err := x.NodeAndLink(ctx, in, "HAS_WHOIS", "whois", record)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if email != "" {
			if _, err := x.NodeAndLink(ctx, w, "REGISTERED_BY", "email", map[string]any{"email": email}); err != nil {
				x.Warn("%s: %v", in.Label, err)
			}
		}

		for kind, c := range map[string]*whoisparser.Contact{
			"registrant": info.Registrant, "administrative": info.Administrative, "technical": info.Technical,
		} {
			if c != nil {
				whoisx.Link(ctx, x, in, whoisx.Contact{Kind: kind, Name: c.Name, Company: c.Organization, Email: c.Email,
					Phone: c.Phone, Address: c.Street, City: c.City, Zip: c.PostalCode, Country: c.Country})
			}
		}
	}

	return nil
}

// DomainToHistory reads a domain's WHOIS history from Whoxy.
type DomainToHistory struct{ whoxy.Client }

// Meta describes the transform.
func (DomainToHistory) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_history", Category: "Domain", Input: "domain", Output: "domain",
		Description: "Read the WHOIS history of a domain and the contacts it names (Whoxy).",
		Params:      []transform.Param{whoxy.Key}}
}

// Run enriches each domain.
func (e DomainToHistory) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		records, err := e.Query(ctx, x, url.Values{"history": {props.Str(in.Properties, "domain")}})
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, r := range records {
			target := in
			if r.DomainName != "" && !strings.EqualFold(r.DomainName, in.Label) {
				if target, err = x.NodeAndLink(ctx, in, "HAS_RELATED_DOMAIN", "domain", map[string]any{"domain": r.DomainName}); err != nil {
					continue
				}
			}

			for _, c := range r.Contacts() {
				whoisx.Link(ctx, x, target, c)
			}
		}
	}

	return nil
}

// DomainToWhoisHistory reads WHOIS history from WhoisXML.
type DomainToWhoisHistory struct {
	BaseURL string
}

// Meta describes the transform.
func (DomainToWhoisHistory) Meta() transform.Meta {
	return transform.Meta{Name: "domain_to_whois_history", Category: "Domain", Input: "domain", Output: "whois",
		Description: "Read the WHOIS history records of a domain (WhoisXML API, uses purchase credits).",
		Params: []transform.Param{{Name: "WHOISXML_API_KEY", Type: transform.ParamSecret, Required: true,
			Description: "The WhoisXML API key for WHOIS history lookups."}}}
}

// whoisXMLRecord is one history record as WhoisXML answers it.
type whoisXMLRecord struct {
	DomainName         string `json:"domainName"`
	RegistrarName      string `json:"registrarName"`
	CreatedDateISO8601 string `json:"createdDateISO8601"`
	ExpiresDateISO8601 string `json:"expiresDateISO8601"`
	RegistrantContact  struct {
		Name         string `json:"name"`
		Organization string `json:"organization"`
		Email        string `json:"email"`
		Telephone    string `json:"telephone"`
		Street       string `json:"street"`
		City         string `json:"city"`
		PostalCode   string `json:"postalCode"`
		Country      string `json:"country"`
	} `json:"registrantContact"`
}

// Run enriches each domain. mode=purchase is sent explicitly: without
// it WhoisXML answers a preview holding only a record count.
func (e DomainToWhoisHistory) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		q := url.Values{"apiKey": {x.Param("WHOISXML_API_KEY")}, "domainName": {props.Str(in.Properties, "domain")},
			"mode": {"purchase"}, "outputFormat": {"JSON"}}

		var answer struct {
			Records []whoisXMLRecord `json:"records"`
		}

		if _, err := httpx.GetJSON(ctx, x, e.BaseURL+"?"+q.Encode(), nil, &answer); err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, r := range answer.Records {
			rc := r.RegistrantContact
			record := map[string]any{"domain": map[string]any{"domain": cmp.Or(r.DomainName, in.Label)},
				"registrar": r.RegistrarName, "creation_date": r.CreatedDateISO8601,
				"expiration_date": r.ExpiresDateISO8601, "city": rc.City, "country": rc.Country}
			if !whoisx.Redacted(rc.Organization) {
				record["organization"] = map[string]any{"name": rc.Organization}
			}

			if email, contactURL := whoisx.Email(rc.Email); email != "" {
				record["email"] = map[string]any{"email": email}
			} else if contactURL != "" {
				record["contact_url"] = contactURL
			}

			if _, err := x.NodeAndLink(ctx, in, "HAS_WHOIS", "whois", record); err != nil {
				x.Warn("%s: %v", in.Label, err)

				continue
			}

			whoisx.Link(ctx, x, in, whoisx.Contact{Kind: "registrant", Name: rc.Name, Company: rc.Organization, Email: rc.Email,
				Phone: rc.Telephone, Address: rc.Street, City: rc.City, Zip: rc.PostalCode, Country: rc.Country})
		}
	}

	return nil
}

// ctxDialer dials under ctx and closes the connection when ctx ends, so
// a cancelled run does not wait out a WHOIS server's answer.
type ctxDialer struct {
	ctx context.Context
	d   *net.Dialer
}

func (c ctxDialer) Dial(network, addr string) (net.Conn, error) {
	conn, err := c.d.DialContext(c.ctx, network, addr)
	if err != nil {
		return nil, err
	}

	stop := context.AfterFunc(c.ctx, func() { _ = conn.Close() })

	return stoppingConn{Conn: conn, stop: stop}, nil
}

// stoppingConn drops the close-on-cancel hook when it is closed.
type stoppingConn struct {
	net.Conn

	stop func() bool
}

func (c stoppingConn) Close() error {
	c.stop()

	return c.Conn.Close()
}
