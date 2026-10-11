// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// A credential is a login (username at a service) or a bare key: an API
// key, a token, a JWT, an SSH key, a private key. Either way the secret
// itself is never stored, a pasted one becomes its fingerprint.

var (
	// keyRes are the shapes of secrets a paste is recognized by, each
	// strict enough to take nothing else.
	keyRes = []struct {
		re   *regexp.Regexp
		kind string
	}{
		{regexp.MustCompile(`^AKIA[0-9A-Z]{16}$`), "api_key"},
		{regexp.MustCompile(`^AIza[0-9A-Za-z_-]{35}$`), "api_key"},
		{regexp.MustCompile(`^sk-[A-Za-z0-9_-]{20,}$`), "api_key"},
		{regexp.MustCompile(`^gh[pousr]_[A-Za-z0-9]{36,}$`), "token"},
		{regexp.MustCompile(`^github_pat_[A-Za-z0-9_]{22,}$`), "token"},
		{regexp.MustCompile(`^glpat-[A-Za-z0-9_-]{20,}$`), "token"},
		{regexp.MustCompile(`^xox[abprs]-[A-Za-z0-9-]{10,}$`), "token"},
		{regexp.MustCompile(`^eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`), "jwt"},
		{regexp.MustCompile(`(?s)^-----BEGIN [A-Z ]*PRIVATE KEY-----`), "private_key"},
		{regexp.MustCompile(`^(?:ssh-(?:rsa|dss|ed25519)|ecdsa-sha2-nistp(?:256|384|521)|sk-(?:ssh-ed25519|ecdsa-sha2-nistp256)@openssh\.com) AAAA[A-Za-z0-9+/=]+(?: \S.*)?$`), "ssh_key"},
	}
)

// fingerprintLen is how much of the sha256 names a secret.
const fingerprintLen = 12

func securityTypes() []spec {
	return []spec{
		{
			name: "credential", title: "Credential", category: "security", icon: "Key", color: "#D4B030",
			before: credentialBefore,
			label: func(p map[string]any) string {
				if truthy(p["username"]) {
					if truthy(p["service"]) {
						return str(p, "username") + "@" + pyStr(p["service"])
					}

					return str(p, "username")
				}

				var parts []string
				for _, k := range []string{"service", "kind"} {
					if truthy(p[k]) {
						parts = append(parts, pyStr(p[k]))
					}
				}

				if truthy(p["prefix"]) {
					parts = append(parts, pyStr(p["prefix"])+"...")
				}

				if len(parts) == 0 {
					return str(p, "fingerprint")
				}

				return strings.Join(parts, " ")
			},
			fromString: func(s string) (map[string]any, error) {
				s = pyStrip(s)
				if keyKind(s) != "" {
					return map[string]any{"value": s}, nil
				}

				return map[string]any{"username": s}, nil
			},
			detect: func(s string) bool { return keyKind(s) != "" },
			key:    credentialKey,
		},
		{
			name: "session", title: "Session", category: "security", icon: "Clock", color: "#A8BF50",
			label:      joined(" - ", "session_id", "user_id", "service"),
			fromString: into("session_id"),
		},
		{
			name: "device", title: "Device", category: "security", icon: "Smartphone", color: "#E3A857",
			label:      joined(" ", "device_id", "manufacturer", "model"),
			fromString: into("device_id"),
		},
		{
			name: "malware", title: "Malware", category: "security", icon: "Bug", color: "#4AA29E",
			label: func(p map[string]any) string {
				if truthy(p["family"]) {
					return str(p, "name") + " (" + pyStr(p["family"]) + ")"
				}

				return str(p, "name")
			},
			fromString: into("name"),
		},
		{
			name: "weapon", title: "Weapon", category: "security", icon: "Sword", color: "#E98973",
			label: func(p map[string]any) string {
				parts := []string{str(p, "name")}
				if truthy(p["type"]) {
					parts = append(parts, "("+pyStr(p["type"])+")")
				}

				return strings.Join(parts, " ")
			},
			fromString: into("name"),
		},
		{
			name: "reputationscore", title: "Reputation Score", category: "security", icon: "Star", color: "#6FA8DC",
			label: func(p map[string]any) string {
				label := str(p, "entity_id")
				if p["score"] != nil {
					label += " - Score: " + pyStr(p["score"])
				}

				return label
			},
			fromString: into("entity_id"),
		},
		{
			name: "riskprofile", title: "Risk Profile", category: "security", icon: "AlertTriangle", color: "#D97474",
			label: func(p map[string]any) string {
				label := str(p, "entity_id")
				if truthy(p["risk_level"]) {
					label += " - Risk: " + pyStr(p["risk_level"])
				}

				return label
			},
			fromString: into("entity_id"),
		},
	}
}

// credentialBefore fingerprints a pasted key and keeps it as value. The
// key arrives as "value", "secret", or in the username field when it was
// typed in as the label. The fingerprint is the identity, the value is
// encrypted at rest by the graph store.
func credentialBefore(p map[string]any) error {
	raw := cmp.Or(str(p, "value"), str(p, "secret"))
	if raw == "" {
		if u := str(p, "username"); keyKind(u) != "" {
			raw = u
			delete(p, "username")
		}
	}

	delete(p, "secret")
	if raw == "" {
		return nil
	}

	p["value"] = raw

	sum := sha256.Sum256([]byte(raw))
	p["fingerprint"] = hex.EncodeToString(sum[:])[:fingerprintLen]
	if !truthy(p["prefix"]) {
		p["prefix"] = prefixOf(raw)
	}

	if !truthy(p["kind"]) {
		if k := keyKind(raw); k != "" {
			p["kind"] = k
		}
	}

	return nil
}

// keyKind is the kind of secret s looks like, "" for none.
func keyKind(s string) string {
	s = pyStrip(s)
	for _, k := range keyRes {
		if k.re.MatchString(s) {
			return k.kind
		}
	}

	return ""
}

// prefixOf is the first four characters of a secret, enough to tell
// which one it was without giving it away. An SSH key is told by its
// algorithm instead.
func prefixOf(s string) string {
	if algo, _, found := strings.Cut(s, " "); found && (strings.HasPrefix(algo, "ssh-") || strings.HasPrefix(algo, "ecdsa-") || strings.HasPrefix(algo, "sk-")) {
		return algo
	}

	r := []rune(s)
	if len(r) <= 4 {
		return ""
	}

	return string(r[:4])
}

// credentialKey is the identity: the fingerprint of a key, else the
// login at its service, so root on ssh and root on the web console
// are two entities and the same leaked key found twice is one.
func credentialKey(p map[string]any) string {
	if fp := str(p, "fingerprint"); fp != "" {
		return fp
	}

	if svc := str(p, "service"); svc != "" {
		return str(p, "username") + "@" + svc
	}

	return str(p, "username")
}
