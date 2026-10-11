// Gosint, Copyright (c) 2026 YouSysAdmin

// Package dehashed is DeHashed's search API: breach records by any of
// the fields they hold (email, username, phone, IP, domain, name).
package dehashed

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the search endpoint.
const URL = "https://api.dehashed.com/v2/search"

// maxResults bounds one search.
const maxResults = 10000

// Public is the client of the public instance.
var Public = Client{BaseURL: URL}

// Params are what every DeHashed transform asks for.
var Params = []transform.Param{
	{Name: "DEHASHED_API_KEY", Type: transform.ParamSecret, Required: true,
		Description: "Your DeHashed API key, from dehashed.com/api."},
	{Name: "max_results", Type: transform.ParamNumber, Default: "100",
		Description: "Most records to read per entity, at most " + strconv.Itoa(maxResults)},
}

// Client talks to one DeHashed.
type Client struct {
	BaseURL string
}

// Entry is one breach record, every field a list.
type Entry struct {
	Email        []string `json:"email"`
	Name         []string `json:"name"`
	Username     []string `json:"username"`
	Phone        []string `json:"phone"`
	IPAddress    []string `json:"ip_address"`
	DOB          []string `json:"dob"`
	Social       []string `json:"social"`
	Password     []string `json:"password"`
	Hash         []string `json:"hashed_password"`
	DatabaseName string   `json:"database_name"`
}

// Run searches field for each input's value and draws what each record
// holds beside the input: the breach with the login it leaked (password
// or hash) as a credential, and every leaked fact as its own entity, so
// the same phone leaked twice is one node.
func (c Client) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node, field string, value func(graphmodel.Node) string) error {
	size := 100
	if n, err := strconv.ParseFloat(x.Param("max_results"), 64); err == nil {
		size = int(n)
	}

	size = min(max(size, 1), maxResults)
	header := http.Header{"Dehashed-Api-Key": {x.Param("DEHASHED_API_KEY")}}
	for _, in := range inputs {
		v := strings.TrimSpace(value(in))
		if v == "" {
			continue
		}

		var answer struct {
			Balance int     `json:"balance"`
			Total   int     `json:"total"`
			Entries []Entry `json:"entries"`
		}

		query := map[string]any{"query": field + ":" + quote(v), "page": 1, "size": size}
		if _, err := httpx.PostJSON(ctx, x, c.BaseURL, header, query, &answer); err != nil {
			var se httpx.StatusError
			if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
				// Every other input would fail the same way.
				return errors.New("DeHashed refused the key: the subscription is not active or the key is wrong")
			}

			x.Warn("%s: %v", in.Label, err)

			continue
		}

		for _, entry := range answer.Entries {
			draw(ctx, x, in, v, entry)
		}

		x.Info("%s: %d breach records of %d, %d search credits left", in.Label, len(answer.Entries), answer.Total, answer.Balance)
	}

	return nil
}

// quote is v as a quoted search term.
func quote(v string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v) + `"`
}

// draw writes one record. A value the type refuses is logged and
// skipped by the Exec, the input itself is never linked to itself.
func draw(ctx context.Context, x *transform.Exec, in graphmodel.Node, searched string, entry Entry) {
	edge := map[string]any{}

	// The credential hangs from the breach that leaked it, from the
	// input when the record names no breach.
	holder := in
	if entry.DatabaseName != "" {
		edge["breach"] = entry.DatabaseName
		breach, err := x.NodeAndLink(ctx, in, "FOUND_IN_BREACH", "breach", map[string]any{"name": entry.DatabaseName})
		if err != nil {
			x.Warn("%s: %v", entry.DatabaseName, err)
		} else {
			holder = breach
		}
	}

	if c := credentialOf(searched, entry); c != nil {
		if _, err := x.NodeAndLink(ctx, holder, "HAS_CREDENTIAL", "credential", c); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	link := func(typeName string, p map[string]any) {
		n, err := x.Node(ctx, typeName, p)
		if err != nil {
			x.Warn("%s: %v", in.Label, err)

			return
		}

		if n.ID == in.ID {
			return
		}

		if err := x.LinkWith(ctx, in, n, "LEAKED_WITH", edge); err != nil {
			x.Warn("%s: %v", in.Label, err)
		}
	}

	for i, name := range entry.Name {
		if name = strings.TrimSpace(name); name == "" {
			continue
		}

		p := map[string]any{"full_name": name}
		if i < len(entry.DOB) && strings.TrimSpace(entry.DOB[i]) != "" {
			p["birth_date"] = strings.TrimSpace(entry.DOB[i])
		}

		link("individual", p)
	}

	each := func(values []string, typeName, key string) {
		for _, v := range values {
			if v = strings.TrimSpace(v); v != "" {
				link(typeName, map[string]any{key: v})
			}
		}
	}

	each(lower(entry.Email), "email", "email")
	each(entry.Username, "username", "value")
	each(entry.Social, "username", "value")
	each(entry.Phone, "phone", "number")
	each(entry.IPAddress, "ip", "address")
}

// credentialOf is the login a record leaked at its breach, with its
// password and hash, nil when the record holds neither. The login is
// the record's email, else its username, else what was searched.
func credentialOf(searched string, entry Entry) map[string]any {
	password, hash := first(entry.Password), first(entry.Hash)
	if password == "" && hash == "" {
		return nil
	}

	login := strings.ToLower(first(entry.Email))
	if login == "" {
		login = first(entry.Username)
	}

	if login == "" {
		login = searched
	}

	c := map[string]any{"username": login, "kind": "password"}
	if entry.DatabaseName != "" {
		c["service"] = entry.DatabaseName
		c["found_in"] = entry.DatabaseName
	}

	if password != "" {
		c["password"] = password
	}

	if hash != "" {
		c["password_hash"] = hash
	}

	return c
}

// first is the first non-blank value of a list.
func first(values []string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}

	return ""
}

func lower(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(v)
	}

	return out
}
