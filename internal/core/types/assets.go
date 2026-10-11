// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"net/url"
	"regexp"
	"strings"
)

// Types for what an investigator holds in hand. A repository with the
// key to read it, a host as one thing, and the physical world. Found
// keys and secrets are credentials (security.go).

var (
	// gitHostRe is a repository URL on a forge, with or without .git.
	gitHostRe = regexp.MustCompile(`^https?://(?:www\.)?(?:github\.com|gitlab\.com|bitbucket\.org|codeberg\.org|sr\.ht)/[\w.-]+/[\w.-]+?(?:\.git)?/?$`)

	// gitURLRe is any http(s) URL ending in .git.
	gitURLRe = regexp.MustCompile(`^https?://[\w.-]+(?::\d+)?/\S+\.git/?$`)

	// gitSCPRe is the ssh shorthand, git@host:owner/name.git.
	gitSCPRe = regexp.MustCompile(`^(?:ssh://)?git@([\w.-]+)[:/]([\w.-]+/[\w./-]+?)(?:\.git)?/?$`)

	// vaultKeyNameRe is validation's key name rule, copied since types
	// must not import validation.
	vaultKeyNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
)

func assetTypes() []spec {
	return []spec{
		{
			name: "gitrepo", title: "Git Repository", category: "network", icon: "GitBranch", color: "#E8734A",
			after: gitRepoAfter,
			label: func(p map[string]any) string {
				if truthy(p["owner"]) && truthy(p["name"]) {
					return str(p, "owner") + "/" + str(p, "name")
				}

				return strings.TrimPrefix(strings.TrimPrefix(str(p, "url"), "https://"), "http://")
			},
			fromString: func(s string) (map[string]any, error) {
				return map[string]any{"url": gitHTTPS(pyStrip(s))}, nil
			},
			detect: func(s string) bool {
				return gitHostRe.MatchString(s) || gitURLRe.MatchString(s) || gitSCPRe.MatchString(s)
			},
		},
		{
			name: "host", title: "Host", category: "network", icon: "Server", color: "#7FA7C9",
			label:      field("hostname"),
			fromString: into("hostname"),
		},
		{
			name: "vehicle", title: "Vehicle", category: "global", icon: "Car", color: "#B07D62",
			label: func(p map[string]any) string {
				var parts []string
				for _, k := range []string{"make", "model"} {
					if truthy(p[k]) {
						parts = append(parts, pyStr(p[k]))
					}
				}

				if len(parts) == 0 {
					return str(p, "plate")
				}

				return str(p, "plate") + " (" + strings.Join(parts, " ") + ")"
			},
			fromString: into("plate"),
		},
		{
			name: "photo", title: "Photo", category: "files", icon: "Image", color: "#8F9CA3",
			before: photoBefore,
			label:  field("title"),
			key: func(p map[string]any) string {
				// A photo is the same photo by its file, else by where it is.
				if h := str(p, "hash"); h != "" {
					return strings.ToLower(h)
				}

				if u := str(p, "url"); u != "" {
					return u
				}

				return str(p, "title")
			},
			fromString: into("title"),
		},
		{
			name: "event", title: "Event", category: "global", icon: "CalendarDays", color: "#6AA84F",
			label: func(p map[string]any) string {
				if truthy(p["starts_at"]) {
					return str(p, "title") + " (" + pyStr(p["starts_at"]) + ")"
				}

				return str(p, "title")
			},
			fromString: into("title"),
		},
	}
}

// photoBefore takes a URL typed in as the title for the photo's address
// and names the photo after the file, so a pasted link just works.
func photoBefore(p map[string]any) error {
	t := pyStrip(str(p, "title"))
	if !truthy(p["url"]) && (strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://")) {
		p["url"] = t
		p["title"] = photoTitle(t)
	}

	return nil
}

// photoTitle names a photo after the last part of its URL, or its host.
func photoTitle(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}

	if base := strings.TrimSuffix(u.Path, "/"); strings.LastIndex(base, "/") >= 0 && base[strings.LastIndex(base, "/")+1:] != "" {
		return base[strings.LastIndex(base, "/")+1:]
	}

	return u.Hostname()
}

// gitHTTPS turns the ssh shorthand into the https URL of the same
// repository, so one form is stored, and leaves anything else as it is.
func gitHTTPS(s string) string {
	m := gitSCPRe.FindStringSubmatch(s)
	if m == nil {
		return s
	}

	return "https://" + m[1] + "/" + m[2]
}

// gitRepoAfter fills host, owner and name from the URL when they are
// empty, and checks the credential names a secret, never holds one.
func gitRepoAfter(p map[string]any) error {
	if u, err := url.Parse(str(p, "url")); err == nil {
		if !truthy(p["host"]) {
			p["host"] = u.Hostname()
		}

		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) >= 2 {
			if !truthy(p["owner"]) {
				p["owner"] = parts[0]
			}

			if !truthy(p["name"]) {
				p["name"] = strings.TrimSuffix(parts[len(parts)-1], ".git")
			}
		}
	}

	if k := str(p, "credential_key"); k != "" && !vaultKeyNameRe.MatchString(k) {
		return fieldErr("gitrepo", "credential_key", "must be the NAME of a secret, letters, digits, _ . -")
	}

	return nil
}
