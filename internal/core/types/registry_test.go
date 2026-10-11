// Gosint, Copyright (c) 2026 YouSysAdmin

package types_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/types"
)

func TestAllCoversEveryType(t *testing.T) {
	all := types.Builtin().All()
	if len(all) != 48 {
		t.Fatalf("%d types, want 48", len(all))
	}

	known := map[string]bool{}
	for _, c := range types.Categories {
		known[c.Key] = true
	}

	seen := map[string]bool{}
	for _, ty := range all {
		if ty.Title == "" || !known[ty.Category] || ty.Description == "" || len(ty.Fields) == 0 {
			t.Errorf("%s: title %q category %q description %q, %d fields", ty.Name, ty.Title, ty.Category,
				ty.Description, len(ty.Fields))
		}

		if seen[ty.Name] {
			t.Errorf("%s listed twice", ty.Name)
		}

		seen[ty.Name] = true

		if ty.Label == nil || ty.Normalize == nil {
			t.Errorf("%s has no Label or Normalize", ty.Name)
		}
	}
}

func TestGetIgnoresCase(t *testing.T) {
	for _, name := range []string{"domain", "Domain", "DOMAIN", "CryptoWallet"} {
		if _, ok := types.Builtin().Get(name); !ok {
			t.Errorf("Get(%q) found nothing", name)
		}
	}

	if _, ok := types.Builtin().Get("nope"); ok {
		t.Error("Get found an unknown type")
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"AS15169", "asn"},
		{"as15169", "asn"},
		{"AS0", "username"},
		{"8.8.8.0/24", "cidr"},
		{"8.8.8.8/24", "cidr"},
		{"2001:db8::/32", "cidr"},
		{"4111 1111 1111 1111", "creditcard"},
		{"4111-1111-1111-1111", "creditcard"},
		{"example.com", "domain"},
		{"sub.example.co.uk", "domain"},
		{"a@example.com", "email"},
		{"d41d8cd98f00b204e9800998ecf8427e", "file"},
		{"da39a3ee5e6b4b0d3255bfef95601890afd80709", "file"},
		{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "file"},
		{"8.8.8.8", "ip"},
		{"::1", "ip"},
		{"+33612345678", "phone"},
		{"2025550123", "phone"},
		{"0612345678", "phone"},
		{"443", "port"},
		{"65535", "port"},
		{"65536", "username"},
		{"0x52908400098527886E0F7030069857D2E4169EE7", "cryptowallet"},
		{"1BoatSLRHtKNngkdXEeobR76b53LETtpyT", "cryptowallet"},
		{"bc1qar0srrr7xfkvy5l643lydnw9re59gtzzwf5mdq", "cryptowallet"},
		{"LVg2kJoFNg45Nbpy53h7Fe1wKyeXVRhMH9", "cryptowallet"},
		{"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", "cryptowallet"},
		{"@alice_99", "username"},
		{"alice-99", "username"},
		{"https://example.com", "website"},
		{"https://example.com/path?q=1", "website"},
		{"https://github.com/acme/tool", "gitrepo"},
		{"https://git.example.com/group/sub/tool.git", "gitrepo"},
		{"git@github.com:acme/tool.git", "gitrepo"},
		{"https://github.com", "website"},
		{"AKIAIOSFODNN7EXAMPLE", "credential"},
		{"cve-2021-44228", "cve"},
		{"ghp_" + strings.Repeat("a", 36), "credential"},
		{"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk", "credential"},
		{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb5n7X7Y2xkz4M1k2x3Q5f6v7w8y9z0A1b2C3d4E5F6 jane@laptop", "credential"},
		{"John Smith", "individual"},
		{"Jean Paul Sartre", "individual"},
		{"  padded.example.com  ", "domain"},
	}
	for _, c := range cases {
		ty, ok := types.Builtin().Detect(c.in)
		if !ok {
			t.Errorf("Detect(%q) found nothing, want %s", c.in, c.want)

			continue
		}

		if ty.Name != c.want {
			t.Errorf("Detect(%q) = %s, want %s", c.in, ty.Name, c.want)
		}
	}
}

func TestDetectFindsNothing(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"a b c d",
		"ab",
		"4111 1111 1111 1112",
		"foo..bar.com",
		"8.8.8.8/33",
		"ftp://example.com",
	} {
		if ty, ok := types.Builtin().Detect(in); ok {
			t.Errorf("Detect(%q) = %s, want nothing", in, ty.Name)
		}
	}
}

func TestParseBuildsNormalizedProps(t *testing.T) {
	cases := []struct {
		in, typ, field string
		want           any
	}{
		{"AS15169", "asn", "asn_str", "AS15169"},
		{"8.8.8.8/24", "cidr", "network", "8.8.8.0/24"},
		{"Example.COM", "domain", "domain", "example.com"},
		{"d41d8cd98f00b204e9800998ecf8427E", "file", "hash_md5", "d41d8cd98f00b204e9800998ecf8427e"},
		{"0612345678", "phone", "number", "+33612345678"},
		{"8080", "port", "number", int64(8080)},
		{"https://Example.com", "website", "url", "https://example.com/"},
		{"@alice_99", "username", "value", "alice_99"},
		{"John  Smith", "individual", "full_name", "John Smith"},
	}
	for _, c := range cases {
		ty, props, err := types.Builtin().Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)

			continue
		}

		if ty.Name != c.typ || props[c.field] != c.want {
			t.Errorf("Parse(%q) = %s %v, want %s.%s = %v", c.in, ty.Name, props, c.typ, c.field, c.want)
		}
	}
}

func TestParseRefusesWhatNothingDetects(t *testing.T) {
	if _, _, err := types.Builtin().Parse("a b c d e"); err == nil {
		t.Error("an unrecognized line parsed")
	}
}

func TestBuildRefusesAnUnknownType(t *testing.T) {
	if _, err := types.Builtin().Build("nope", map[string]any{}); err == nil {
		t.Error("an unknown type built")
	}
}

func TestBuildNamesTheBadField(t *testing.T) {
	_, err := types.Builtin().Build("ip", map[string]any{"address": "999.1.1.1"})

	fe, ok := errors.AsType[*types.FieldError](err)
	if !ok || fe.Field != "address" || fe.Type != "ip" {
		t.Fatalf("err = %v, want a FieldError on ip.address", err)
	}
}

// NaN and the infinities parse as floats but cannot be stored as JSON,
// so a float field refuses them like any other non-number.
func TestAFloatFieldRefusesNaNAndInfinity(t *testing.T) {
	for _, v := range []any{"NaN", "nan", "Inf", "-Infinity", "1e400"} {
		_, err := types.Builtin().Build("ip", map[string]any{"address": "1.1.1.1", "latitude": v})

		fe, ok := errors.AsType[*types.FieldError](err)
		if !ok || fe.Field != "latitude" {
			t.Errorf("latitude %v: err = %v, want a FieldError on latitude", v, err)
		}
	}
}

func TestBuildRequiresRequiredFields(t *testing.T) {
	_, err := types.Builtin().Build("breach", map[string]any{"description": "no name"})

	fe, ok := errors.AsType[*types.FieldError](err)
	if !ok || fe.Field != "name" {
		t.Fatalf("err = %v, want name required", err)
	}
}

// The computed label wins over a supplied one, which is used only when
// the props give none, and then fills the empty primary field.
func TestTheComputedLabelWins(t *testing.T) {
	props := map[string]any{"domain": "example.com", "label": "something else"}

	label, err := types.Builtin().Build("domain", props)
	if err != nil || label != "example.com" {
		t.Errorf("label = %q, %v", label, err)
	}

	if _, kept := props["label"]; kept {
		t.Error("the label key stayed in the props")
	}

	props = map[string]any{"nodeLabel": "Agent X", "age": 40.0}

	label, err = types.Builtin().Build("individual", props)
	if err != nil || label != "Agent X" || props["full_name"] != "Agent X" || props["age"] != int64(40) {
		t.Errorf("label %q props %v err %v", label, props, err)
	}

	if _, err := types.Builtin().Build("individual", map[string]any{}); !errors.Is(err, types.ErrNoLabel) {
		t.Errorf("no names and no label: %v, want ErrNoLabel", err)
	}
}

func TestUnknownKeysAreKept(t *testing.T) {
	props := map[string]any{"address": "1.1.1.1", "seen_by": "shodan"}
	if _, err := types.Builtin().Build("ip", props); err != nil {
		t.Fatal(err)
	}

	if props["seen_by"] != "shodan" {
		t.Error("an unknown key was dropped")
	}
}

// A pasted key is kept as the value beside its fingerprint, which stays
// the identity, and the value is a field the store seals.
func TestACredentialKeepsTheKey(t *testing.T) {
	r := types.Builtin()
	const key = "AKIAIOSFODNN7EXAMPLE" //nolint:gosec // AWS's documented example key
	p := map[string]any{"label": key}
	if _, err := r.Build("credential", p); err != nil {
		t.Fatal(err)
	}

	if p["value"] != key || p["fingerprint"] == "" || r.Key("credential", p) != p["fingerprint"] {
		t.Errorf("credential = %v key %q", p, r.Key("credential", p))
	}

	if got := r.SecretFields("credential"); !slices.Equal(got, []string{"value", "password", "password_hash"}) {
		t.Errorf("secret fields = %q", got)
	}
}

// A CVE id is written one way, a finding and a vulnerability are one
// entity per place, and both label with where they are.
func TestWeaknessTypes(t *testing.T) {
	r := types.Builtin()

	cve := map[string]any{"id": " cve-2021-44228 "}
	if _, err := r.Build("cve", cve); err != nil || cve["id"] != "CVE-2021-44228" {
		t.Errorf("cve = %v, %v", cve, err)
	}

	if _, err := r.Build("cve", map[string]any{"id": "log4shell"}); err == nil {
		t.Error("a CVE id that is not one passed")
	}

	a := map[string]any{"title": "Exposed .git", "rule": "git-config", "location": "https://a.example/.git/config"}
	b := map[string]any{"title": "Exposed .git", "rule": "git-config", "location": "https://b.example/.git/config"}
	label, err := r.Build("finding", a)
	if err != nil || label != "Exposed .git @ a.example/.git/config" || a["status"] != "unverified" {
		t.Errorf("finding = %q %v %v", label, a, err)
	}

	if _, err := r.Build("finding", b); err != nil || r.Key("finding", a) == r.Key("finding", b) {
		t.Errorf("two places share a key: %q", r.Key("finding", a))
	}

	v := map[string]any{"name": "SQL injection", "affected": "https://a.example/login", "cvss": 9.8}
	if label, err := r.Build("vulnerability", v); err != nil || label != "SQL injection @ a.example/login" || v["status"] != "open" {
		t.Errorf("vulnerability = %q %v %v", label, v, err)
	}

	if _, err := r.Build("vulnerability", map[string]any{"name": "x", "cvss": 11.0}); err == nil {
		t.Error("a CVSS over 10 passed")
	}
}

// A card number is kept as digits, its scheme named, a wrong check
// digit refused, a masked number left alone.
func TestCreditCardNumbers(t *testing.T) {
	r := types.Builtin()
	for _, tc := range []struct{ number, brand string }{
		{"4111 1111 1111 1111", "Visa"},
		{"5555-5555-5555-4444", "Mastercard"},
		{"2223000048400011", "Mastercard"},
		{"378282246310005", "American Express"},
		{"6011111111111117", "Discover"},
		{"3530111333300000", "JCB"},
		{"30569309025904", "Diners Club"},
		{"6200000000000005", "UnionPay"},
	} {
		p := map[string]any{"card_number": tc.number}
		if _, err := r.Build("creditcard", p); err != nil || p["card_type"] != tc.brand || !types.Truthy(p["card_number"]) {
			t.Errorf("%s = %v, %v, want %s", tc.number, p, err, tc.brand)
		}
	}

	if _, err := r.Build("creditcard", map[string]any{"card_number": "4111 1111 1111 1112"}); err == nil {
		t.Error("a number failing the Luhn check passed")
	}

	kept := map[string]any{"card_number": "4111 11** **** 1111", "card_type": "Visa"}
	if _, err := r.Build("creditcard", kept); err != nil || kept["card_number"] != "4111 11** **** 1111" {
		t.Errorf("a masked number = %v, %v", kept, err)
	}

	own := map[string]any{"card_number": "4111111111111111", "card_type": "Visa Debit"}
	if _, err := r.Build("creditcard", own); err != nil || own["card_type"] != "Visa Debit" {
		t.Errorf("a card type the person set = %v", own)
	}
}
