// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"strings"
	"testing"
	"time"
)

type stubCtx []byte

func (s stubCtx) Body() []byte { return s }

type decodeInner struct {
	Ports []int `json:"ports"`
}

type decodeInput struct {
	Name    string            `json:"name"`
	Tags    []string          `json:"tags"`
	Count   int               `json:"count"`
	Enabled *bool             `json:"enabled"`
	At      time.Time         `json:"at"`
	Labels  map[string]string `json:"labels"`
	Inner   decodeInner       `json:"inner"`
}

// A body of the wrong JSON type names the key the caller sent and says
// what it should have been, in words and not in Go types.
func TestADecodeErrorNamesTheKeyAndNoGoType(t *testing.T) {
	cases := []struct {
		body, field, rule, message string
	}{
		{`{"name":5}`, "name", "type", "Name must be a string"},
		{`{"tags":"x"}`, "tags", "type", "Tags must be a list"},
		{`{"tags":[1]}`, "tags", "type", "Tags must be a string"},
		{`{"count":"x"}`, "count", "type", "Count must be a whole number"},
		{`{"count":1.5}`, "count", "type", "Count must be a whole number"},
		{`{"count":99999999999999999999}`, "count", "type", "Count is out of range"},
		{`{"enabled":"yes"}`, "enabled", "type", "Enabled must be true or false"},
		{`{"at":5}`, "at", "type", "At must be a date and time (RFC 3339)"},
		{`{"labels":{"k":1}}`, "labels", "type", "Labels must be a string"},
		{`{"inner":{"ports":[1,"x"]}}`, "ports", "type", "Ports must be a whole number"},
		{`{"name":"a","name":"b"}`, "name", "duplicate", "Name is given more than once"},
		{`[]`, "", "type", "Request body must be a JSON object"},
		{`5`, "", "type", "Request body must be a JSON object"},
		{`{"name":`, "", "json", "Request body is not valid JSON"},
		{``, "", "required", "Request body is required"},
		{"  \n", "", "required", "Request body is required"},
	}

	for _, c := range cases {
		_, err := BindAndValidate[decodeInput](stubCtx(c.body))
		if err == nil {
			t.Errorf("%s: decoded, want an error", c.body)

			continue
		}

		fes := Humanize(err)
		if len(fes) != 1 {
			t.Fatalf("%s: %d field errors, want 1", c.body, len(fes))
		}

		got := fes[0]
		if got.Field != c.field || got.Rule != c.rule || got.Message != c.message {
			t.Errorf("%s: got {%q %q %q}, want {%q %q %q}", c.body, got.Field, got.Rule, got.Message, c.field, c.rule, c.message)
		}

		for _, leak := range []string{"Go ", "[]string", "int", "decodeInput", "json:"} {
			if strings.Contains(got.Message, leak) {
				t.Errorf("%s: message %q leaks %q", c.body, got.Message, leak)
			}
		}
	}
}

// The summary of several errors is one line with no semicolon.
func TestTheSummaryCarriesNoSemicolon(t *testing.T) {
	got := Summary([]FieldError{{Message: "Name is required"}, {Message: "Port must be at least 1"}})
	if strings.Contains(got, ";") {
		t.Errorf("summary %q carries a semicolon", got)
	}

	if !strings.Contains(got, "Name is required") || !strings.Contains(got, "Port must be at least 1") {
		t.Errorf("summary %q lost a message", got)
	}
}
