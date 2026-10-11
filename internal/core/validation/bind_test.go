// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"maps"
	"slices"
	"testing"
)

type ontoInput struct {
	Name    string            `json:"name"    validate:"required" normalize:"trim"`
	Note    string            `json:"note"`
	Rate    int               `json:"rate"`
	Headers map[string]string `json:"headers"`
	Tags    []string          `json:"tags"`
}

// A partial update keeps what the body leaves out, replaces a map or a
// list the body names instead of merging into it, and is validated as
// the whole it becomes.
func TestDecodeOntoKeepsWhatTheBodyLeavesOut(t *testing.T) {
	base := ontoInput{
		Name: "Launch", Note: "keep me", Rate: 5,
		Headers: map[string]string{"X-A": "1", "X-B": "2"},
		Tags:    []string{"a", "b"},
	}

	got, err := decodeOnto([]byte(`{"name":"  Relaunch ","headers":{"X-C":"3"}}`), base)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Name != "Relaunch" || got.Note != "keep me" || got.Rate != 5 {
		t.Errorf("scalars = %+v, want the name edited and the rest kept", got)
	}

	if !maps.Equal(got.Headers, map[string]string{"X-C": "3"}) {
		t.Errorf("headers = %v, want the named map to replace the stored one", got.Headers)
	}

	if !slices.Equal(got.Tags, []string{"a", "b"}) {
		t.Errorf("tags = %v, want the unnamed list kept", got.Tags)
	}

	if !maps.Equal(base.Headers, map[string]string{"X-A": "1", "X-B": "2"}) {
		t.Errorf("the base map was written through: %v", base.Headers)
	}

	if _, err := decodeOnto([]byte(`{"name":"  "}`), base); err == nil {
		t.Error("a blanked required field was accepted")
	}

	if _, err := decodeOnto([]byte(`{"rate":"x"}`), base); err == nil {
		t.Error("a mistyped field was accepted")
	}

	if _, err := decodeOnto([]byte(``), base); err == nil {
		t.Error("an empty body was accepted")
	}
}
