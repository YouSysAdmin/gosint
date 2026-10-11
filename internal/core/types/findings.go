// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"regexp"
	"strings"
)

// The weakness types, for a hand or a scanner report alike. A FINDING is
// something noticed that still needs checking, a VULNERABILITY a
// weakness confirmed (a finding is linked to it once checked), and a
// CVE the public record either may name.

// cveRe is a CVE id, CVE-year-sequence, the sequence four digits or more.
var cveRe = regexp.MustCompile(`(?i)^\s*cve-(\d{4})-(\d{4,})\s*$`)

// Severities is the scale finding, vulnerability and cve share.
var Severities = []string{"info", "low", "medium", "high", "critical"}

func findingTypes() []spec {
	return []spec{
		{
			name: "finding", title: "Finding", category: "security", icon: "ScanSearch", color: "#E0A243",
			label:      placed("title", "location"),
			key:        placedKey("rule", "title", "location"),
			fromString: into("title"),
		},
		{
			name: "vulnerability", title: "Vulnerability", category: "security", icon: "ShieldAlert", color: "#D9534F",
			before:     vulnerabilityBefore,
			label:      placed("name", "affected"),
			key:        placedKey("", "name", "affected"),
			fromString: into("name"),
		},
		{
			name: "cve", title: "CVE", category: "security", icon: "FileWarning", color: "#B5525C",
			before: cveBefore,
			label:  field("id"),
			fromString: func(s string) (map[string]any, error) {
				return map[string]any{"id": s}, nil
			},
			detect: func(s string) bool { return cveRe.MatchString(s) },
		},
	}
}

// placed labels a weakness by what it is and, when known, where:
// "Exposed .git @ example.com/.git".
func placed(what, where string) func(map[string]any) string {
	return func(p map[string]any) string {
		label := str(p, what)
		at := strings.TrimPrefix(strings.TrimPrefix(str(p, where), "https://"), "http://")
		if at == "" {
			return label
		}

		if r := []rune(at); len(r) > 60 {
			at = string(r[:57]) + "..."
		}

		return label + " @ " + at
	}
}

// placedKey is a weakness's identity: the same rule (or, without one,
// the same name) at the same place is the same thing seen again, while
// the same name at two places is two.
func placedKey(rule, what, where string) func(map[string]any) string {
	return func(p map[string]any) string {
		id := str(p, what)
		if rule != "" && str(p, rule) != "" {
			id = str(p, rule)
		}

		if at := str(p, where); at != "" {
			return id + "@" + at
		}

		return id
	}
}

// vulnerabilityBefore writes a CVE it names as CVE-YYYY-NNNN.
func vulnerabilityBefore(p map[string]any) error {
	if str(p, "cve") == "" {
		return nil
	}

	id := CVEID(str(p, "cve"))
	if id == "" {
		return fieldErr("vulnerability", "cve", "must read CVE-YYYY-NNNN")
	}

	p["cve"] = id

	return nil
}

// CVEID is id written as CVE-YYYY-NNNN, "" when it is not a CVE id.
func CVEID(id string) string {
	m := cveRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}

	return "CVE-" + m[1] + "-" + m[2]
}

// cveBefore writes the id as CVE-YYYY-NNNN and refuses anything else.
func cveBefore(p map[string]any) error {
	m := cveRe.FindStringSubmatch(str(p, "id"))
	if m == nil {
		return fieldErr("cve", "id", "must read CVE-YYYY-NNNN")
	}

	p["id"] = "CVE-" + m[1] + "-" + m[2]

	return nil
}
