// Gosint, Copyright (c) 2026 YouSysAdmin

// Package whoxy is the Whoxy API: WHOIS history and reverse WHOIS.
package whoxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/whoisx"
)

// URL is the API root.
const URL = "https://api.whoxy.com/"

// Key is the secret every Whoxy transform asks for.
var Key = transform.Param{Name: "WHOXY_API_KEY", Type: transform.ParamSecret, Required: true,
	Description: "The Whoxy API key."}

// Public is the client of the public instance.
var Public = Client{BaseURL: URL}

// Client talks to one Whoxy.
type Client struct {
	BaseURL string
}

// Contact is a contact as Whoxy answers it.
type Contact struct {
	FullName       string `json:"full_name"`
	CompanyName    string `json:"company_name"`
	EmailAddress   string `json:"email_address"`
	PhoneNumber    string `json:"phone_number"`
	MailingAddress string `json:"mailing_address"`
	CityName       string `json:"city_name"`
	ZipCode        string `json:"zip_code"`
	CountryName    string `json:"country_name"`
}

// Record is one WHOIS record as Whoxy answers it.
type Record struct {
	DomainName     string   `json:"domain_name"`
	Registrant     *Contact `json:"registrant_contact"`
	Administrative *Contact `json:"administrative_contact"`
	Technical      *Contact `json:"technical_contact"`
	Billing        *Contact `json:"billing_contact"`
}

// Contacts lists the record's contacts, each with its kind.
func (r Record) Contacts() []whoisx.Contact {
	var out []whoisx.Contact
	for kind, c := range map[string]*Contact{
		"registrant": r.Registrant, "administrative": r.Administrative, "technical": r.Technical, "billing": r.Billing,
	} {
		if c != nil {
			out = append(out, whoisx.Contact{Kind: kind, Name: c.FullName, Company: c.CompanyName, Email: c.EmailAddress,
				Phone: c.PhoneNumber, Address: c.MailingAddress, City: c.CityName, Zip: c.ZipCode, Country: c.CountryName})
		}
	}

	return out
}

// Query asks Whoxy, retrying a rate limit with backoff. Records come
// from whois_records (history) or search_result (reverse).
func (c Client) Query(ctx context.Context, x *transform.Exec, params url.Values) ([]Record, error) {
	params.Set("key", x.Param(Key.Name))

	var answer struct {
		Status        int      `json:"status"`
		StatusReason  string   `json:"status_reason"`
		WhoisRecords  []Record `json:"whois_records"`
		SearchResults []Record `json:"search_result"`
	}

	var err error
	for attempt := range 4 {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<attempt) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		_, err = httpx.GetJSON(ctx, x, c.BaseURL+"?"+params.Encode(), nil, &answer)
		if se, ok := errors.AsType[httpx.StatusError](err); err == nil || !ok || se.Code != http.StatusTooManyRequests {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	if answer.Status != 1 {
		return nil, fmt.Errorf("whoxy: %s", answer.StatusReason)
	}

	return append(answer.WhoisRecords, answer.SearchResults...), nil
}

// Reverse links what the records name: from -label-> each domain, and
// the contacts of each.
func (c Client) Reverse(ctx context.Context, x *transform.Exec, from graphmodel.Node, params url.Values, label string) {
	params.Set("reverse", "whois")
	records, err := c.Query(ctx, x, params)
	if err != nil {
		x.Warn("%s: %v", from.Label, err)

		return
	}

	for _, r := range records {
		d, err := x.NodeAndLink(ctx, from, label, "domain", map[string]any{"domain": r.DomainName})
		if err != nil {
			x.Warn("%s: %s: %v", from.Label, r.DomainName, err)

			continue
		}

		for _, contact := range r.Contacts() {
			whoisx.Link(ctx, x, d, contact)
		}
	}
}
