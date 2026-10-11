// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"testing"

	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

func TestTheTypeCatalog(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var body struct {
		Categories []struct {
			Key string `json:"key"`
		} `json:"categories"`
		Types []struct {
			Name   string `json:"name"`
			Fields []struct {
				Name    string `json:"name"`
				Primary bool   `json:"primary"`
			} `json:"fields"`
		} `json:"types"`
	}

	alice.expect(http.StatusOK, "GET", "/api/types", nil, &body)
	if len(body.Types) != 48 || len(body.Categories) == 0 {
		t.Fatalf("%d types, %d categories", len(body.Types), len(body.Categories))
	}

	for _, ty := range body.Types {
		if ty.Name == "domain" && (len(ty.Fields) == 0 || !ty.Fields[0].Primary) {
			t.Errorf("domain fields = %+v", ty.Fields)
		}
	}
}

func TestDetectingValues(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var body struct {
		Results []struct {
			Value      string         `json:"value"`
			Type       string         `json:"type"`
			Label      string         `json:"label"`
			Properties map[string]any `json:"properties"`
		} `json:"results"`
	}

	alice.expect(http.StatusOK, "POST", "/api/types/detect", map[string]any{"values": []string{
		" example.com ", "8.8.8.8", "a@example.com", "AS15169", "!!!",
		"0x52908400098527886E0F7030069857D2E4169EE7",
	}}, &body)

	want := []string{"domain", "ip", "email", "asn", "phrase", "cryptowallet"}
	if len(body.Results) != len(want) {
		t.Fatalf("results = %+v", body.Results)
	}

	for i, r := range body.Results {
		if r.Type != want[i] {
			t.Errorf("%q detected as %s, want %s", r.Value, r.Type, want[i])
		}
	}

	if body.Results[0].Value != "example.com" || body.Results[0].Properties["domain"] != "example.com" {
		t.Errorf("domain result = %+v", body.Results[0])
	}
}
