// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/transforms/domain"
	"github.com/yousysadmin/gosint/internal/transforms/email"
)

func TestWhoxyReverseWhoisLinksContacts(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("key") != "k" || r.URL.Query().Get("reverse") != "whois" {
				_, _ = w.Write([]byte(`{"status":0,"status_reason":"bad key"}`))

				return
			}

			_, _ = w.Write([]byte(`{"status":1,"total_results":1,"search_result":[{"domain_name":"acme.io",
				"registrant_contact":{"full_name":"Jane Roe","company_name":"Acme Ltd","email_address":"jane@acme.io",
				"phone_number":"+33612345678"},
				"technical_contact":{"full_name":"REDACTED FOR PRIVACY","email_address":"REDACTED FOR PRIVACY"}}]}`))
		},
	})

	h.run(email.EmailToDomains{BaseURL: base}, map[string]string{"WHOXY_API_KEY": "k"},
		h.node("email", map[string]any{"email": "jane@acme.io"}))

	got := h.edges()
	for _, want := range []string{
		"jane@acme.io -HAS_REGISTERED_DOMAIN-> acme.io",
		"Jane Roe -IS_REGISTRANT_CONTACT-> acme.io",
		"Jane Roe -HAS_EMAIL-> jane@acme.io",
		"Jane Roe -HAS_PHONE-> +33612345678",
		"Jane Roe -WORKS_FOR-> Acme Ltd",
		"Acme Ltd -HAS_DOMAIN-> acme.io",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}

	if slices.ContainsFunc(got, func(e string) bool { return strings.Contains(e, "REDACTED") }) {
		t.Errorf("a redacted contact made it into the graph: %q", got)
	}
}

func TestWhoxyHistoryReportsAnAPIRefusal(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":0,"status_reason":"Incorrect API Key"}`))
		},
	})

	h.run(domain.DomainToHistory{BaseURL: base}, map[string]string{"WHOXY_API_KEY": "bad"},
		h.node("domain", map[string]any{"domain": "acme.io"}))

	if len(h.edges()) != 0 || !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "Incorrect API Key") }) {
		t.Errorf("edges %q log %q", h.edges(), h.log)
	}
}

// Registrars hiding a contact put a contact form URL in its email field,
// which must not cost the whole WHOIS record.
func TestAContactFormURLIsNotAnEmail(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"records":[{"domainName":"sysadmin.host","registrarName":"Cloudflare, Inc.",
				"registrantContact":{"name":"REDACTED FOR PRIVACY",
				"email":"https://domaincontact.registrar.cloudflare.com/sysadmin.host"}}]}`))
		},
	})

	in := h.node("domain", map[string]any{"domain": "sysadmin.host"})
	h.run(domain.DomainToWhoisHistory{BaseURL: base}, map[string]string{"WHOISXML_API_KEY": "k"}, in)

	if got := h.edges(); !slices.Contains(got, "sysadmin.host -HAS_WHOIS-> sysadmin.host") {
		t.Fatalf("the WHOIS record was dropped: edges %q, log %q", got, h.log)
	}

	if slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "not a valid email") }) {
		t.Errorf("the contact URL was read as an email: %q", h.log)
	}
}
