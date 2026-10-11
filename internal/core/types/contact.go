// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import "regexp"

var emailDetectRe = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

func contactTypes() []spec {
	return []spec{
		{
			name: "email", title: "Email", category: "contact", icon: "Mail", color: "#8E7CC3",
			label:      field("email"),
			fromString: into("email"),
			detect:     emailDetectRe.MatchString,
		},
		{
			name: "phone", title: "Phone", category: "contact", icon: "Phone", color: "#4CB5AE",
			after: func(p map[string]any) error {
				n, ok := normalizePhone(str(p, "number"))
				if !ok {
					return fieldErr("phone", "number",
						"invalid phone number: %s, use the international format (+...) or a national one", str(p, "number"))
				}

				p["number"] = n

				return nil
			},
			label:      field("number"),
			fromString: into("number"),
			detect:     detectPhone,
		},
		{
			name: "message", title: "Message", category: "contact", icon: "MessageSquare", color: "#897FC9",
			label: func(p map[string]any) string {
				if truthy(p["subject"]) {
					return pyStr(p["subject"])
				}

				content := str(p, "content")
				if len([]rune(content)) > 50 {
					return firstRunes(content, 50) + "..."
				}

				return content
			},
		},
	}
}
