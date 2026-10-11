// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"strings"
	"testing"
)

func TestFriendlyField(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Override map cases: tricky abbreviations + non-obvious names
		// users see by their label, not their json tag.
		{"id", "ID"},
		// Auto-titlecase fallback for plain snake_case fields.
		{"name", "Name"},
		{"priority", "Priority"},
		{"some_new_field", "Some new field"},
		{"smtp_server_id", "SMTP server ID"},
		{"allowed_ips", "Allowed IPs"},
		// Empty / malformed inputs degrade gracefully.
		{"", "This field"},
	}
	for _, c := range cases {
		got := friendlyField(c.in)
		if got != c.want {
			t.Errorf("friendlyField(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// A length says what it counts. "Password must be at least 12" left the
// reader to guess, and the same tag bounds a number, which takes no unit.
func TestASizeRuleNamesItsUnit(t *testing.T) {
	type input struct {
		Password string   `json:"password" validate:"min=12"`
		Tags     []string `json:"tags" validate:"max=1"`
		Port     int      `json:"port" validate:"min=1"`
	}

	got := map[string]string{}
	for _, fe := range Humanize(V().Struct(input{Password: "short", Tags: []string{"a", "b"}})) {
		got[fe.Field] = fe.Message
	}

	for field, want := range map[string]string{
		"password": "Password must be at least 12 characters",
		"tags":     "Tags must be at most 1 item",
		"port":     "Port must be at least 1",
	} {
		if got[field] != want {
			t.Errorf("%s: %q, want %q", field, got[field], want)
		}
	}
}

// A password is bounded by bytes, what bcrypt counts: forty Cyrillic
// letters are eighty bytes and refused with a field message, thirty
// are sixty and pass.
func TestMaxBytesCountsBytes(t *testing.T) {
	type input struct {
		Password string `json:"password" validate:"maxbytes=72"`
	}

	long := Humanize(V().Struct(input{Password: strings.Repeat("\u0436", 40)}))
	if len(long) != 1 || long[0].Message != "Password must be at most 72 bytes" {
		t.Errorf("80 bytes = %+v", long)
	}

	if err := V().Struct(input{Password: strings.Repeat("\u0436", 30)}); err != nil {
		t.Errorf("60 bytes refused: %v", err)
	}
}
