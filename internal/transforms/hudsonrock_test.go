// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"net/http"
	"testing"

	"github.com/yousysadmin/gosint/internal/transforms/email"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
	"github.com/yousysadmin/gosint/internal/transforms/phone"
	"github.com/yousysadmin/gosint/internal/transforms/username"
)

// Each input gets its own computers, never every device under every
// input, the stealer is drawn, a masked address is not, and the
// phone search tries the number without its plus sign.
func TestInfostealers(t *testing.T) {
	answer := `{"message": "found", "stealers": [{"computer_name": "DESKTOP-7Q1", "operating_system": "Windows 10 Pro x64",
		"date_compromised": "2023-05-01T10:00:00.000Z", "stealer_family": "RedLine", "malware_path": "C:\\Users\\x\\a.exe",
		"ip": "203.0.113.9", "antiviruses": ["Windows Defender"], "top_logins": ["j****@example.com"], "top_passwords": ["hu*****"],
		"total_user_services": 12, "total_corporate_services": 1},
		{"computer_name": "Not Found", "date_compromised": "2022-01-02T00:00:00.000Z", "stealer_family": "Raccoon", "ip": "198.***.***.4"}]}`
	none := `{"message": "not associated", "stealers": []}`
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/search-by-email": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("email") == "jane@example.com" {
				_, _ = w.Write([]byte(answer))

				return
			}

			_, _ = w.Write([]byte(none))
		},
		"/search-by-username": func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Query().Get("username") {
			case "jroe", "33612345678":
				_, _ = w.Write([]byte(answer))
			default:
				_, _ = w.Write([]byte(none))
			}
		},
	})
	client := hudsonrock.Client{BaseURL: base}
	found := []string{
		"DESKTOP-7Q1 2023-05-01 -INFECTED_BY-> RedLine",
		"DESKTOP-7Q1 2023-05-01 -HAS_IP-> 203.0.113.9",
		"infected computer 2022-01-02 -INFECTED_BY-> Raccoon",
	}

	h := newHarness(t)
	h.run(email.EmailToInfostealers{Client: client}, nil,
		h.node("email", map[string]any{"email": "jane@example.com"}),
		h.node("email", map[string]any{"email": "clean@example.com"}))
	h.expectEdges(append([]string{
		"jane@example.com -ASSOCIATED_WITH_DEVICE-> DESKTOP-7Q1 2023-05-01",
		"jane@example.com -ASSOCIATED_WITH_DEVICE-> infected computer 2022-01-02",
	}, found...)...)

	nodes, _, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	for _, n := range nodes {
		if n.Label == "DESKTOP-7Q1 2023-05-01" && (n.Properties["os"] != "Windows 10 Pro x64" || n.Properties["source"] != "RedLine stealer") {
			t.Errorf("device = %v", n.Properties)
		}
	}

	u := newHarness(t)
	u.run(username.UsernameToInfostealers{Client: client}, nil, u.node("username", map[string]any{"value": "jroe"}))
	u.expectEdges(append([]string{
		"jroe -ASSOCIATED_WITH_DEVICE-> DESKTOP-7Q1 2023-05-01",
		"jroe -ASSOCIATED_WITH_DEVICE-> infected computer 2022-01-02",
	}, found...)...)

	p := newHarness(t)
	p.run(phone.PhoneToInfostealers{Client: client}, nil, p.node("phone", map[string]any{"number": "+33612345678"}))
	p.expectEdges(append([]string{
		"+33612345678 -ASSOCIATED_WITH_DEVICE-> DESKTOP-7Q1 2023-05-01",
		"+33612345678 -ASSOCIATED_WITH_DEVICE-> infected computer 2022-01-02",
	}, found...)...)
}
