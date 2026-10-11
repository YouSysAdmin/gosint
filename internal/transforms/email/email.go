// Gosint, Copyright (c) 2026 YouSysAdmin

// Package email holds the transforms that start from an email address.
package email

import (
	"context"
	"crypto/md5" //nolint:gosec // gravatar addresses profiles by md5
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/whoxy"
)

const (
	// GravatarURL is Gravatar's root.
	GravatarURL = "https://www.gravatar.com/"

	// HIBPURL is Have I Been Pwned's breached account endpoint.
	HIBPURL = "https://haveibeenpwned.com/api/v3/breachedaccount/"
)

// EmailToDomain links an email address to its domain.
type EmailToDomain struct{}

// Meta describes the transform.
func (EmailToDomain) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_domain", Category: "Email", Input: "email", Output: "domain",
		Description: "Extract the domain of an email address."}
}

// Run enriches each email.
func (EmailToDomain) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		_, domain, ok := strings.Cut(props.Str(in.Properties, "email"), "@")
		if !ok || domain == "" {
			continue
		}

		if _, err := x.NodeAndLink(ctx, in, "HAS_DOMAIN", "domain", map[string]any{"domain": domain}); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// EmailToUsername links an email address to its local part.
type EmailToUsername struct{}

// Meta describes the transform.
func (EmailToUsername) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_username", Category: "Email", Input: "email", Output: "username",
		Description: "Extract the username part of an email address."}
}

// Run enriches each email.
func (EmailToUsername) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		user, _, ok := strings.Cut(props.Str(in.Properties, "email"), "@")
		if !ok || user == "" {
			continue
		}

		if _, err := x.NodeAndLink(ctx, in, "HAS_USERNAME", "username", map[string]any{"value": user}); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// EmailToGravatar links an email to its Gravatar profile, if any.
type EmailToGravatar struct {
	BaseURL string
}

// Meta describes the transform.
func (EmailToGravatar) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_gravatar", Category: "Email", Input: "email", Output: "gravatar",
		Description: "Find the Gravatar avatar and public profile of an email address."}
}

// gravatarProfile is the part of a Gravatar profile kept.
type gravatarProfile struct {
	Entry []struct {
		DisplayName     string `json:"displayName"`
		AboutMe         string `json:"aboutMe"`
		CurrentLocation string `json:"currentLocation"`
	} `json:"entry"`
}

// Run enriches each email.
func (e EmailToGravatar) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		sum := md5.Sum([]byte(strings.ToLower(props.Str(in.Properties, "email")))) //nolint:gosec // see import
		hash := hex.EncodeToString(sum[:])
		avatar := e.BaseURL + "avatar/" + hash + "?d=404"

		exists, err := httpx.Head(ctx, x, avatar)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !exists {
			continue
		}

		profileURL := e.BaseURL + hash + ".json"
		profile := map[string]any{"src": avatar, "hash": hash, "profile_url": profileURL, "exists": true}

		var p gravatarProfile
		if found, err := httpx.GetJSON(ctx, x, profileURL, nil, &p); err == nil && found && len(p.Entry) > 0 {
			profile["display_name"] = p.Entry[0].DisplayName
			profile["about_me"] = p.Entry[0].AboutMe
			profile["location"] = p.Entry[0].CurrentLocation
		}

		if _, err := x.NodeAndLink(ctx, in, "HAS_GRAVATAR", "gravatar", profile); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	return nil
}

// EmailToBreaches links an email to the breaches Have I Been Pwned knows it from.
type EmailToBreaches struct{}

// Meta describes the transform.
func (EmailToBreaches) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_breaches", Category: "Email", Input: "email", Output: "breach",
		Description: "Find the data breaches an email address appears in (Have I Been Pwned).",
		Params: []transform.Param{
			{Name: "HIBP_API_KEY", Type: transform.ParamSecret, Required: true,
				Description: "The HIBP API key to use for breaches lookup."},
			{Name: "HIBP_API_URL", Type: transform.ParamURL, Default: HIBPURL,
				Description: "The HIBP API URL to use for breaches lookup."},
		}}
}

// hibpBreach is one breach as HIBP answers it.
type hibpBreach struct {
	Name               string   `json:"Name"`
	Title              string   `json:"Title"`
	Domain             string   `json:"Domain"`
	BreachDate         string   `json:"BreachDate"`
	AddedDate          string   `json:"AddedDate"`
	ModifiedDate       string   `json:"ModifiedDate"`
	PwnCount           int64    `json:"PwnCount"`
	Description        string   `json:"Description"`
	LogoPath           string   `json:"LogoPath"`
	DataClasses        []string `json:"DataClasses"`
	IsVerified         bool     `json:"IsVerified"`
	IsFabricated       bool     `json:"IsFabricated"`
	IsSensitive        bool     `json:"IsSensitive"`
	IsRetired          bool     `json:"IsRetired"`
	IsSpamList         bool     `json:"IsSpamList"`
	IsMalware          bool     `json:"IsMalware"`
	IsStealerLog       bool     `json:"IsStealerLog"`
	IsSubscriptionFree bool     `json:"IsSubscriptionFree"`
}

// Run enriches each email.
func (EmailToBreaches) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	header := http.Header{"Hibp-Api-Key": {x.Param("HIBP_API_KEY")}}
	base := x.Param("HIBP_API_URL")
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	for _, in := range inputs {
		var breaches []hibpBreach
		// The URL is a parameter, so the guarded client.
		found, err := httpx.GetJSONWeb(ctx, x, base+url.PathEscape(props.Str(in.Properties, "email"))+"?truncateResponse=false", header, &breaches)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			continue
		}

		if !found {
			continue
		}

		for _, b := range breaches {
			classes := make([]any, 0, len(b.DataClasses))
			for _, c := range b.DataClasses {
				classes = append(classes, c)
			}

			breach := map[string]any{
				"name": b.Name, "title": b.Title, "domain": b.Domain, "breachdate": b.BreachDate,
				"addeddate": b.AddedDate, "modifieddate": b.ModifiedDate, "pwncount": float64(b.PwnCount),
				"description": b.Description, "logopath": b.LogoPath, "dataclasses": classes,
				"isverified": b.IsVerified, "isfabricated": b.IsFabricated, "issensitive": b.IsSensitive,
				"isretired": b.IsRetired, "isspamlist": b.IsSpamList, "ismalware": b.IsMalware,
				"isstealerlog": b.IsStealerLog, "issubscriptionfree": b.IsSubscriptionFree,
			}
			if _, err := x.NodeAndLink(ctx, in, "FOUND_IN_BREACH", "breach", breach); err != nil {
				x.Warn("%s: %s: %v", in.Label, b.Name, err)
			}
		}
	}

	return nil
}

// EmailToDomains finds the domains registered with an email (Whoxy).
type EmailToDomains struct{ whoxy.Client }

// Meta describes the transform.
func (EmailToDomains) Meta() transform.Meta {
	return transform.Meta{Name: "email_to_domains", Category: "Email", Input: "email", Output: "domain",
		Description: "Find the domains registered with an email address (Whoxy reverse WHOIS).",
		Params:      []transform.Param{whoxy.Key}}
}

// Run enriches each email.
func (e EmailToDomains) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		e.Reverse(ctx, x, in, url.Values{"email": {props.Str(in.Properties, "email")}}, "HAS_REGISTERED_DOMAIN")
	}

	return nil
}

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		EmailToDomain{},
		EmailToUsername{},
		EmailToGravatar{BaseURL: GravatarURL},
		EmailToBreaches{},
		EmailToDehashed{Client: dehashed.Public},
		EmailToDomains{Client: whoxy.Public},
		EmailToInfostealers{Client: hudsonrock.Public},
	}
}
