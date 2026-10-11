// Gosint, Copyright (c) 2026 YouSysAdmin

package types_test

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/types"
)

// Every type's label rule, pinned.
func TestEveryTypeComputesItsLabel(t *testing.T) {
	long := strings.Repeat("x", 120)
	for _, tc := range []struct {
		name  string
		typ   string
		props map[string]any
		want  string
	}{
		{"location", "location", map[string]any{"address": "1 Main St", "city": "Paris", "country": "FR", "zip": "75001"}, "1 Main St, Paris, FR"},
		{"phrase", "phrase", map[string]any{"text": "hello"}, "hello"},
		{"long phrase is cut", "phrase", map[string]any{"text": long}, long[:100] + "..."},
		{"affiliation with role", "affiliation", map[string]any{"organization": "ACME", "role": "CTO"}, "CTO at ACME"},
		{"affiliation", "affiliation", map[string]any{"organization": "ACME"}, "ACME"},
		{"alias", "alias", map[string]any{"alias": "neo"}, "neo"},
		{"individual full name", "individual", map[string]any{"full_name": "John Doe"}, "John Doe"},
		{"individual first and last", "individual", map[string]any{"first_name": "John", "last_name": "Doe"}, "John Doe"},
		{"individual first only", "individual", map[string]any{"first_name": "John"}, "John"},
		{"username", "username", map[string]any{"value": "jdoe"}, "jdoe"},
		{"organization", "organization", map[string]any{"name": "ACME"}, "ACME"},
		{"email", "email", map[string]any{"email": "a@example.com"}, "a@example.com"},
		{"gravatar display name", "gravatar", map[string]any{"hash": "abc", "display_name": "Jo", "src": "https://gravatar.com/avatar/abc"}, "Jo"},
		{"gravatar hash", "gravatar", map[string]any{"hash": "abc", "src": "https://gravatar.com/avatar/abc"}, "abc"},
		{"phone", "phone", map[string]any{"number": "+33612345678"}, "+33612345678"},
		{"asn with name", "asn", map[string]any{"asn_str": "AS15169", "name": "GOOGLE"}, "AS15169 - GOOGLE"},
		{"cidr", "cidr", map[string]any{"network": "10.0.0.0/8"}, "10.0.0.0/8"},
		{"domain", "domain", map[string]any{"domain": "example.com"}, "example.com"},
		{"ip", "ip", map[string]any{"address": "8.8.8.8"}, "8.8.8.8"},
		{"port", "port", map[string]any{"number": 443.0, "service": "https", "protocol": "tcp"}, "443 https (tcp)"},
		{"technology with version", "technology", map[string]any{"name": "nginx", "version": "1.25"}, "nginx 1.25"},
		{"sslcertificate", "sslcertificate", map[string]any{"subject": "CN=example.com"}, "CN=example.com"},
		{"credential with service", "credential", map[string]any{"username": "root", "service": "ssh"}, "root@ssh"},
		{"device model", "device", map[string]any{"device_id": "d1", "manufacturer": "Apple", "model": "iPhone"}, "Apple iPhone"},
		{"device id", "device", map[string]any{"device_id": "d1"}, "d1"},
		{"malware with family", "malware", map[string]any{"name": "Emotet", "family": "trojan"}, "Emotet (trojan)"},
		{"reputation", "reputationscore", map[string]any{"entity_id": "8.8.8.8", "score": 3.0}, "8.8.8.8 - Score: 3.0"},
		{"risk", "riskprofile", map[string]any{"entity_id": "8.8.8.8", "risk_level": "high"}, "8.8.8.8 - Risk: high"},
		{"session with user", "session", map[string]any{"session_id": "s1", "user_id": "u1", "service": "web"}, "u1 - web"},
		{"session id", "session", map[string]any{"session_id": "s1"}, "s1"},
		{"document", "document", map[string]any{"title": "Report"}, "Report"},
		{"file", "file", map[string]any{"filename": "a.pdf"}, "a.pdf"},
		{"script without name", "script", map[string]any{"script_id": "sc1"}, "sc1"},
		{"bank account", "bankaccount", map[string]any{"account_number": "123456789", "bank_name": "BNP"}, "BNP - ****6789"},
		{"credit card", "creditcard", map[string]any{"card_number": "4111111111111111", "card_type": "Visa"}, "Visa ****1111"},
		{"breach title wins", "breach", map[string]any{"name": "adobe", "title": "Adobe"}, "Adobe"},
		{"leak", "leak", map[string]any{"name": "combo"}, "combo"},
		{"wallet tells its currency", "cryptowallet", map[string]any{"address": "0x52908400098527886E0F7030069857D2E4169EE7"}, "ETH 0x52908400098527886E0F7030069857D2E4169EE7"},
		{"wallet with a given currency", "cryptowallet", map[string]any{"address": "unknownshape000000000000000000", "currency": "sol"}, "SOL unknownshape000000000000000000"},
		{"transaction takes the wallet's currency", "cryptowallettransaction", map[string]any{"hash": "0xabc", "source": map[string]any{"address": "1BoatSLRHtKNngkdXEeobR76b53LETtpyT"}}, "0xabc"},
		{"gitrepo from url", "gitrepo", map[string]any{"url": "https://github.com/acme/tool.git"}, "acme/tool"},
		{"gitrepo bare url", "gitrepo", map[string]any{"url": "https://git.example.com/tool"}, "git.example.com/tool"},
		{"credential from a pasted key", "credential", map[string]any{"value": "AKIAIOSFODNN7EXAMPLE", "service": "AWS"}, "AWS api_key AKIA..."}, //nolint:gosec // AWS's documented example key
		{"credential pasted key as the label", "credential", map[string]any{"label": "AKIAIOSFODNN7EXAMPLE"}, "api_key AKIA..."},                 //nolint:gosec // AWS's documented example key
		{"credential fingerprint only", "credential", map[string]any{"fingerprint": "0123456789ab"}, "0123456789ab"},
		{"credential ssh key by algorithm", "credential", map[string]any{"value": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGb5n7X7Y2xkz4M1k2x3Q5f6v7w8y9z0A1b2C3d4E5F6"}, "ssh_key ssh-ed25519..."},
		{"host", "host", map[string]any{"hostname": "web-01"}, "web-01"},
		{"vehicle with make", "vehicle", map[string]any{"plate": "AB-123-CD", "make": "Renault", "model": "Clio"}, "AB-123-CD (Renault Clio)"},
		{"photo", "photo", map[string]any{"title": "Beach"}, "Beach"},
		{"photo pasted as a link", "photo", map[string]any{"label": "https://example.com/pics/p.jpg"}, "p.jpg"},
		{"event with start", "event", map[string]any{"title": "Launch", "starts_at": "2026-10-06"}, "Launch (2026-10-06)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := types.Builtin().Build(tc.typ, tc.props)
			if err != nil {
				t.Fatalf("Build(%s, %v): %v", tc.typ, tc.props, err)
			}

			if got != tc.want {
				t.Errorf("label = %q, want %q", got, tc.want)
			}
		})
	}
}

// Normalization of the primary field.
func TestPrimaryFieldsAreNormalized(t *testing.T) {
	for _, tc := range []struct {
		typ, field string
		in, want   any
	}{
		{"domain", "domain", "https://Example.com/path", "example.com"},
		{"cidr", "network", "8.8.8.8/24", "8.8.8.0/24"},
		{"ip", "address", "8.8.8.8", "8.8.8.8"},
	} {
		props := map[string]any{tc.field: tc.in}
		if _, err := types.Builtin().Build(tc.typ, props); err != nil {
			t.Errorf("%s %v: %v", tc.typ, tc.in, err)

			continue
		}

		if props[tc.field] != tc.want {
			t.Errorf("%s.%s: %v became %v, want %v", tc.typ, tc.field, tc.in, props[tc.field], tc.want)
		}
	}

	for _, tc := range []struct{ typ, field, in string }{
		{"domain", "domain", "not a domain"},
		{"ip", "address", "999.1.1.1"},
		{"email", "email", "nobody"},
		{"cidr", "network", "10.0.0.0/99"},
	} {
		if _, err := types.Builtin().Build(tc.typ, map[string]any{tc.field: tc.in}); err == nil {
			t.Errorf("%s accepted %q", tc.typ, tc.in)
		}
	}
}

// Every type with a textual form reads back what it was given.
func TestFromStringRoundTrips(t *testing.T) {
	samples := map[string]string{
		"phrase": "free text", "affiliation": "ACME", "alias": "neo", "individual": "John Doe",
		"username": "jdoe", "organization": "ACME", "email": "a@example.com", "phone": "+33612345678",
		"asn": "AS15169", "cidr": "10.0.0.0/8", "domain": "example.com", "ip": "8.8.8.8", "port": "443",
		"sslcertificate": "CN=example.com", "technology": "nginx", "webtracker": "UA-12345-1",
		"website": "https://example.com", "credential": "root", "device": "d1", "malware": "Emotet",
		"reputationscore": "8.8.8.8", "riskprofile": "8.8.8.8", "session": "s1", "weapon": "AK-47",
		"document": "Report", "file": "a.pdf", "script": "sc1", "bankaccount": "FR7612345",
		"creditcard": "4111111111111111", "breach": "adobe", "leak": "combo",
		"cryptowallet": "0x52908400098527886E0F7030069857D2E4169EE7",
		"gitrepo":      "git@github.com:acme/tool.git", "host": "web-01",
		"vehicle": "AB-123-CD", "photo": "Beach", "event": "Launch", "location": "1 Main St",
		"finding": "Exposed .git", "vulnerability": "SQL injection", "cve": "CVE-2021-44228",
	}

	// These label only the last four digits.
	masked := map[string]bool{"bankaccount": true, "creditcard": true}

	// These label something computed from the sample instead.
	relabelled := map[string]string{"gitrepo": "acme/tool", "cryptowallet": "eth 0x52908400098527886e0f7030069857d2e4169ee7"}

	for _, ty := range types.Builtin().All() {
		if ty.FromString == nil {
			continue
		}

		sample, ok := samples[ty.Name]
		if !ok {
			t.Errorf("%s has FromString but no sample in this test", ty.Name)

			continue
		}

		props, err := ty.FromString(sample)
		if err != nil {
			t.Errorf("%s.FromString(%q): %v", ty.Name, sample, err)

			continue
		}

		label, err := types.Builtin().Build(ty.Name, props)
		if err != nil {
			t.Errorf("%s: Build after FromString(%q): %v", ty.Name, sample, err)

			continue
		}

		want := strings.ToLower(strings.TrimPrefix(sample, "https://"))
		if masked[ty.Name] {
			want = want[len(want)-4:]
		}

		if w, ok := relabelled[ty.Name]; ok {
			want = w
		}

		if !strings.Contains(strings.ToLower(label), want) {
			t.Errorf("%s: label %q does not carry %q", ty.Name, label, sample)
		}
	}
}
