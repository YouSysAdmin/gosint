// Gosint, Copyright (c) 2026 YouSysAdmin

// Package expand holds the transform that draws, as entities of their
// own, what an entity's fields name: a person's phones, emails,
// addresses and bank accounts, a device's addresses, a certificate's
// domains, a card's holder. Nothing is looked up, the graph is unfolded.
package expand

import (
	"context"
	"maps"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// rule says which entity a field's value is and how the relationship is
// labelled. into is the target's field, its primary when empty.
type rule struct {
	field, target, into, label string
}

// rules per type, in the order the entities are drawn.
var rules = map[string][]rule{
	"individual": {
		{"phone_numbers", "phone", "", "HAS_PHONE"},
		{"email_addresses", "email", "", "HAS_EMAIL"},
		{"current_address", "location", "", "LIVES_AT"},
		{"previous_addresses", "location", "", "LIVED_AT"},
		{"birth_place", "location", "", "BORN_AT"},
		{"death_place", "location", "", "DIED_AT"},
		{"bank_accounts", "bankaccount", "", "HAS_BANK_ACCOUNT"},
		{"social_media_profiles", "", "", "HAS_SOCIAL_ACCOUNT"},
		{"spouse_name", "individual", "", "MARRIED_TO"},
		{"children_names", "individual", "", "PARENT_OF"},
	},
	"organization": {
		{"website", "website", "", "HAS_WEBSITE"},
		{"email", "email", "", "HAS_EMAIL"},
		{"phone", "phone", "", "HAS_PHONE"},
		{"address", "location", "", "LOCATED_AT"},
		{"executives", "individual", "", "HAS_EXECUTIVE"},
		{"parent", "organization", "", "SUBSIDIARY_OF"},
	},
	"device": {
		{"ip_addresses", "ip", "", "HAS_IP"},
		{"associated_users", "username", "", "USED_BY"},
	},
	"session":                 {{"ip_address", "ip", "", "FROM_IP"}},
	"socialaccount":           {{"username", "username", "", "HAS_USERNAME"}, {"profile_url", "website", "", "HAS_PROFILE_PAGE"}, {"associated_emails", "email", "", "HAS_EMAIL"}, {"associated_phones", "phone", "", "HAS_PHONE"}},
	"sslcertificate":          {{"san_domains", "domain", "", "COVERS"}},
	"dnsrecord":               {{"associated_domains", "domain", "", "HAS_DOMAIN"}, {"nameserver", "domain", "", "HAS_NS"}},
	"website":                 {{"domain", "domain", "", "HAS_DOMAIN"}, {"final_url", "website", "", "REDIRECTS_TO"}},
	"whois":                   {{"domain", "domain", "", "HAS_DOMAIN"}, {"organization", "organization", "", "REGISTERED_BY"}, {"email", "email", "", "REGISTRANT_EMAIL"}},
	"host":                    {{"ip", "ip", "", "HAS_IP"}, {"hostname", "domain", "", "HAS_NAME"}},
	"bankaccount":             {{"account_holder", "individual", "", "HELD_BY"}, {"bank_name", "organization", "", "AT_BANK"}},
	"creditcard":              {{"cardholder_name", "individual", "", "HELD_BY"}, {"associated_accounts", "bankaccount", "", "LINKED_ACCOUNT"}},
	"breach":                  {{"domain", "domain", "", "OF_DOMAIN"}},
	"webtracker":              {{"domain", "domain", "", "OF_DOMAIN"}, {"script_url", "website", "", "SERVED_FROM"}},
	"gitrepo":                 {{"host", "domain", "", "HOSTED_ON"}},
	"credential":              {{"url", "website", "", "SIGNS_IN_AT"}},
	"cryptonft":               {{"wallet", "cryptowallet", "", "OWNED_BY"}},
	"cryptowallettransaction": {{"source", "cryptowallet", "", "FROM_WALLET"}, {"target", "cryptowallet", "", "TO_WALLET"}},
	"event":                   {{"location", "location", "", "HELD_AT"}, {"participants", "individual", "", "ATTENDED_BY"}},
	"document":                {{"source_url", "website", "", "FROM_PAGE"}},
	"script":                  {{"url", "website", "", "SERVED_FROM"}},
	"gravatar":                {{"profile_url", "website", "", "HAS_PROFILE_PAGE"}},
	"asn":                     {{"cidrs", "cidr", "", "ANNOUNCES"}},
}

// Types lists the types the transform does something for.
func Types() []string {
	out := make([]string, 0, len(rules))
	for t := range rules {
		out = append(out, t)
	}

	return out
}

// FieldsToEntities draws the entities an entity's fields name.
type FieldsToEntities struct{}

// Meta describes the transform. Input "" takes any type.
func (FieldsToEntities) Meta() transform.Meta {
	return transform.Meta{Name: "fields_to_entities", Category: "Graph", Output: "",
		Description: "Draw what this entity's fields name as entities of their own: a person's phones, emails, addresses, " +
			"bank accounts and relatives, an organization's site, contacts and executives, a device's addresses, " +
			"a certificate's domains, a card's holder. Nothing is looked up."}
}

// Run unfolds each input.
func (FieldsToEntities) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	reg := types.Builtin()
	for _, in := range inputs {
		set := rules[in.Type]
		if set == nil {
			x.Warn("%s: a %s has no field that names another entity", in.Label, in.Type)

			continue
		}

		drawn := 0
		for _, r := range set {
			for _, v := range values(in.Properties[r.field]) {
				target, props := entity(reg, r, v)
				if target == "" || props == nil {
					continue
				}

				if in.Type == "individual" && target == "bankaccount" && !types.Truthy(props["account_holder"]) {
					props["account_holder"] = in.Label
				}

				n, err := x.NodeAndLink(ctx, in, r.label, target, props)
				if err != nil {
					x.Warn("%s, %s: %v", in.Label, r.field, err)

					continue
				}

				if n.ID != in.ID {
					drawn++
				}
			}
		}

		x.Info("%s: %d entities from its fields", in.Label, drawn)
	}

	return nil
}

// values is a field's content as the list of what it names: one value,
// or each of a list.
func values(v any) []any {
	switch v := v.(type) {
	case nil:
		return nil
	case []any:
		return v
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}

		return []any{v}
	default:
		return []any{v}
	}
}

// entity is the type and properties one value makes: a nested object
// as it is, a string into the target's field. A social profile is a
// website when it is a URL, a username otherwise.
func entity(reg *types.Registry, r rule, v any) (string, map[string]any) {
	target := r.target
	if m, ok := v.(map[string]any); ok {
		if target == "" {
			return "", nil
		}

		return target, maps.Clone(m)
	}

	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", nil
	}

	s = strings.TrimSpace(s)
	if target == "" {
		if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
			return "website", map[string]any{"url": s}
		}

		return "username", map[string]any{"value": strings.TrimPrefix(s, "@")}
	}

	into := r.into
	if into == "" {
		into = primary(reg, target)
	}

	if into == "" {
		return "", nil
	}

	return target, map[string]any{into: s}
}

// primary is the field a type is named by.
func primary(reg *types.Registry, typeName string) string {
	t, ok := reg.Get(typeName)
	if !ok {
		return ""
	}

	for _, f := range t.Fields {
		if f.Primary {
			return f.Name
		}
	}

	return ""
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{FieldsToEntities{}, FilesToEntities{}}
}
