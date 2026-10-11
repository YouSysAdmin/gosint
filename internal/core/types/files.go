// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import "strings"

// hashFields maps a hex digest length to the field it belongs in.
var hashFields = map[int]string{32: "hash_md5", 40: "hash_sha1", 64: "hash_sha256"}

// hashField is the digest field a hex string fits, "" for none.
func hashField(s string) string {
	s = pyStrip(s)
	if !hexRe.MatchString(s) {
		return ""
	}

	return hashFields[len(s)]
}

func fileTypes() []spec {
	return []spec{
		{
			name: "document", title: "Document", category: "files", icon: "FileText", color: "#8F9CA3",
			label:      field("title"),
			fromString: into("title"),
		},
		{
			name: "file", title: "File", category: "files", icon: "File", color: "#8F9CA3",
			label: field("filename"),
			fromString: func(s string) (map[string]any, error) {
				s = pyStrip(s)
				props := map[string]any{"filename": s}
				if hf := hashField(s); hf != "" {
					props[hf] = strings.ToLower(s)
				}

				return props, nil
			},
			detect: func(s string) bool { return hashField(s) != "" },
		},
		{
			name: "script", title: "Script", category: "files", icon: "FileCode", color: "#A36FA3",
			label:      either("name", "script_id"),
			fromString: into("script_id"),
		},
	}
}

func leakTypes() []spec {
	return []spec{
		{
			name: "breach", title: "Breach", category: "leaks", icon: "ShieldAlert", color: "#CC7A7A",
			label:      either("title", "name"),
			fromString: into("name"),
		},
		{
			name: "leak", title: "Leak", category: "leaks", icon: "Droplet", color: "#7C9CBF",
			label:      field("name"),
			fromString: into("name"),
		},
	}
}
