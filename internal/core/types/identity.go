// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"net/netip"
	"regexp"
	"strings"
	"unicode"
)

var (
	usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,80}$`)
	hexRe      = regexp.MustCompile(`^[0-9a-fA-F]+$`)
)

// keepValid filters an Individual's contact list:
// a string or an object is checked by its primary field, and anything
// invalid is dropped rather than refusing the whole entity.
func keepValid(p map[string]any, key, primary string, check func(string) (string, bool)) {
	raw, present := p[key]
	if !present || raw == nil {
		return
	}

	list, ok := toList(raw)
	if !ok {
		return
	}

	var out []any
	for _, e := range list {
		var item map[string]any
		switch x := e.(type) {
		case string:
			item = map[string]any{primary: x}
		case map[string]any:
			item = x
		default:
			continue
		}

		v, ok := item[primary].(string)
		if !ok || v == "" {
			continue
		}

		norm, ok := check(v)
		if !ok {
			continue
		}

		item[primary] = norm
		out = append(out, item)
	}

	if len(out) == 0 {
		p[key] = nil

		return
	}

	p[key] = out
}

// splitFirst splits a trimmed line at its first whitespace run, as
// Python's split(maxsplit=1) does.
func splitFirst(s string) (string, string) {
	i := strings.IndexFunc(s, unicode.IsSpace)
	if i < 0 {
		return s, ""
	}

	return s[:i], strings.TrimLeftFunc(s[i:], unicode.IsSpace)
}

func identityTypes() []spec {
	return []spec{
		{
			name: "location", title: "Location", category: "global", icon: "MapPin", color: "#E57373",
			// Only the address is required: a person's field names a place
			// by its address alone, and a city alone is a place too.
			label:      joined(", ", "address", "address", "city", "country"),
			fromString: into("address"),
		},
		{
			name: "phrase", title: "Phrase", category: "global", icon: "Quote", color: "#BFA77A",
			label: func(p map[string]any) string {
				if p["text"] == nil {
					return ""
				}

				s := pyStr(p["text"])
				if len([]rune(s)) > 100 {
					return firstRunes(s, 100) + "..."
				}

				return s
			},
			fromString: into("text"),
		},
		{
			name: "individual", title: "Individual", category: "person", icon: "User", color: "#4C8EDA",
			before: func(p map[string]any) error {
				keepValid(p, "email_addresses", "email", func(s string) (string, bool) {
					e, err := normalizeEmail(s)

					return e, err == nil
				})
				keepValid(p, "phone_numbers", "number", normalizePhone)
				keepValid(p, "ip_addresses", "address", func(s string) (string, bool) {
					_, err := netip.ParseAddr(s)

					return s, err == nil
				})

				return nil
			},
			after: func(p map[string]any) error {
				first, last := str(p, "first_name"), str(p, "last_name")
				switch {
				case truthy(p["full_name"]):
				case first != "" && last != "":
					p["full_name"] = first + " " + last
				case first != "":
					p["full_name"] = first
				case last != "":
					p["full_name"] = last
				}

				return nil
			},
			label: field("full_name"),
			fromString: func(s string) (map[string]any, error) {
				first, last := splitFirst(pyStrip(s))

				return map[string]any{"first_name": first, "last_name": last}, nil
			},
			detect: func(s string) bool {
				n := len(strings.Split(s, " "))

				return n >= 2 && n <= 3
			},
		},
		{
			name: "organization", title: "Organization", category: "organization", icon: "Building2", color: "#BCA18A",
			label:      field("name"),
			fromString: into("name"),
		},
		{
			name: "username", title: "Username", category: "person", icon: "UserPlus", color: "#8B83C1",
			after: func(p map[string]any) error {
				p["value"] = strings.TrimPrefix(str(p, "value"), "@")

				return nil
			},
			label:      field("value"),
			fromString: into("value"),
			detect: func(s string) bool {
				raw := strings.TrimPrefix(s, "@")
				if raw == "" {
					return false
				}

				// Leave hex digests to File.
				if n := len(raw); (n == 32 || n == 40 || n == 64) && hexRe.MatchString(raw) {
					return false
				}

				return usernameRe.MatchString(raw)
			},
		},
		{
			name: "socialaccount", title: "Social Account", category: "contact", icon: "Share2", color: "#A76DAA",
			before: func(p map[string]any) error {
				v, present := p["username"]
				if !present {
					return nil
				}

				switch x := v.(type) {
				case string:
					p["username"] = map[string]any{"value": strings.TrimPrefix(x, "@")}
				default:
					if !truthy(v) {
						p["username"] = map[string]any{"value": ""}
					}
				}

				return nil
			},
			after: func(p map[string]any) error {
				u, hasUser := p["username"].(map[string]any)
				name := str(u, "value")
				switch {
				case hasUser && truthy(p["platform"]):
					p["id"] = name + "@" + pyStr(p["platform"])
				case hasUser:
					p["id"] = name
				case truthy(p["display_name"]):
					p["id"] = pyStr(p["display_name"])
				default:
					p["id"] = "unknown"
				}

				return nil
			},
			label: func(p map[string]any) string {
				if !truthy(p["display_name"]) {
					return str(p, "id")
				}

				display := pyStr(p["display_name"])
				if u, ok := p["username"].(map[string]any); ok {
					return display + " (@" + str(u, "value") + ")"
				}

				return display
			},
		},
		{
			name: "alias", title: "Alias", category: "person", icon: "UserCog", color: "#A36FA3",
			label:      field("alias"),
			fromString: into("alias"),
		},
		{
			name: "affiliation", title: "Affiliation", category: "person", icon: "Link2", color: "#66A892",
			label: func(p map[string]any) string {
				if truthy(p["role"]) {
					return pyStr(p["role"]) + " at " + str(p, "organization")
				}

				return str(p, "organization")
			},
			fromString: into("organization"),
		},
		{
			name: "gravatar", title: "Gravatar", category: "contact", icon: "UserCircle", color: "#6CB7CA",
			label: either("display_name", "hash"),
		},
	}
}
