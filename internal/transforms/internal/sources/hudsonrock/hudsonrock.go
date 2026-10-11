// Gosint, Copyright (c) 2026 YouSysAdmin

// Package hudsonrock is HudsonRock's free OSINT API: computers infected
// by an infostealer that held a login for an email or a username.
package hudsonrock

import (
	"context"
	"net/url"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the API root.
const URL = "https://cavalier.hudsonrock.com/api/json/v2/osint-tools/"

// Public is the client of the public instance.
var Public = Client{BaseURL: URL}

// Client talks to one HudsonRock.
type Client struct {
	BaseURL string
}

// Stealer is one infected computer as HudsonRock answers it. The free
// API masks part of the IP, the logins and the passwords.
type Stealer struct {
	ComputerName           string   `json:"computer_name"`
	OperatingSystem        string   `json:"operating_system"`
	DateCompromised        string   `json:"date_compromised"`
	StealerFamily          string   `json:"stealer_family"`
	MalwarePath            string   `json:"malware_path"`
	IP                     any      `json:"ip"`
	Antiviruses            any      `json:"antiviruses"`
	TopLogins              []string `json:"top_logins"`
	TopPasswords           []string `json:"top_passwords"`
	TotalUserServices      int      `json:"total_user_services"`
	TotalCorporateServices int      `json:"total_corporate_services"`
}

// Search asks for the infected computers holding value, by "email" or
// "username".
func (c Client) Search(ctx context.Context, x *transform.Exec, by, value string) ([]Stealer, error) {
	var answer struct {
		Stealers []Stealer `json:"stealers"`
	}

	u := c.BaseURL + "search-by-" + by + "?" + url.Values{by: {value}}.Encode()
	if _, err := httpx.GetJSON(ctx, x, u, nil, &answer); err != nil {
		return nil, err
	}

	return answer.Stealers, nil
}

// Lookup searches value and draws each infected computer from in: the
// device under ASSOCIATED_WITH_DEVICE, the stealer that took it under
// INFECTED_BY, its address when the API shows it whole. A device hangs
// under the input it was found for, not under every input of the run.
func (c Client) Lookup(ctx context.Context, x *transform.Exec, in graphmodel.Node, by, value string) {
	stealers, err := c.Search(ctx, x, by, value)
	if err != nil {
		x.Warn("%s: %v", value, err)

		return
	}

	if len(stealers) == 0 {
		x.Info("%s: no infected computer known", value)

		return
	}

	for _, s := range stealers {
		draw(ctx, x, in, s)
	}

	x.Info("%s: %d infected computers", value, len(stealers))
}

func draw(ctx context.Context, x *transform.Exec, in graphmodel.Node, s Stealer) {
	day, _, _ := strings.Cut(s.DateCompromised, "T")
	name := orDefault(s.ComputerName, "infected computer")

	// The computer name alone is often a default (user-PC), the day of
	// the infection tells two machines apart.
	id := strings.TrimSpace(name + " " + day)
	device := map[string]any{"device_id": id, "type": "PC/Laptop", "is_desktop": true, "last_seen": s.DateCompromised}
	if family := orDefault(s.StealerFamily, ""); family != "" {
		device["source"] = family + " stealer"
	}

	set := func(k string, v any) {
		switch v := v.(type) {
		case string:
			if v != "" && !notFound(v) {
				device[k] = v
			}
		case []string:
			if len(v) > 0 {
				device[k] = v
			}
		case int:
			if v > 0 {
				device[k] = v
			}
		}
	}

	set("os", s.OperatingSystem)
	set("malware_path", s.MalwarePath)
	set("antiviruses", list(s.Antiviruses))
	set("top_logins", s.TopLogins)
	set("top_passwords", s.TopPasswords)
	set("total_user_services", s.TotalUserServices)
	set("total_corporate_services", s.TotalCorporateServices)

	ips := whole(list(s.IP))
	if len(ips) > 0 {
		device["ip_addresses"] = ips
	}

	dev, err := x.NodeAndLink(ctx, in, "ASSOCIATED_WITH_DEVICE", "device", device)
	if err != nil {
		x.Warn("%s: %v", id, err)

		return
	}

	if s.StealerFamily != "" && !notFound(s.StealerFamily) {
		if _, err := x.NodeAndLink(ctx, dev, "INFECTED_BY", "malware",
			map[string]any{"name": s.StealerFamily, "type": "infostealer"}); err != nil {
			x.Warn("%s: %v", s.StealerFamily, err)
		}
	}

	for _, ip := range ips {
		if _, err := x.NodeAndLink(ctx, dev, "HAS_IP", "ip", map[string]any{"address": ip}); err != nil {
			x.Warn("%s: %v", ip, err)
		}
	}
}

// list reads a value the API sends as a string or a list of strings.
func list(v any) []string {
	switch v := v.(type) {
	case string:
		if v = strings.TrimSpace(v); v != "" && !notFound(v) {
			return []string{v}
		}
	case []any:
		var out []string
		for _, e := range v {
			if s, ok := e.(string); ok && strings.TrimSpace(s) != "" && !notFound(s) {
				out = append(out, strings.TrimSpace(s))
			}
		}

		return out
	}

	return nil
}

// whole keeps the addresses the free API did not mask.
func whole(ips []string) []string {
	var out []string
	for _, ip := range ips {
		if !strings.Contains(ip, "*") {
			out = append(out, ip)
		}
	}

	return out
}

func notFound(s string) bool { return strings.EqualFold(strings.TrimSpace(s), "not found") }

func orDefault(s, fallback string) string {
	if s = strings.TrimSpace(s); s == "" || notFound(s) {
		return fallback
	}

	return s
}
