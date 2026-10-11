// Gosint, Copyright (c) 2026 YouSysAdmin

package transform_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// The run line names the first inputs, every parameter in order, and a
// secret by its name, never its value.
func TestDescribeNamesInputsAndParams(t *testing.T) {
	meta := transform.Meta{Name: "x", Input: "domain", Params: []transform.Param{
		{Name: "check", Type: transform.ParamSelect},
		{Name: "API_KEY", Type: transform.ParamSecret},
		{Name: "with_subdomains", Type: transform.ParamBool},
	}}

	var inputs []graphmodel.Node
	for _, l := range []string{"a.test", "b.test", "c.test", "d.test", "e.test", "f.test", "g.test"} {
		inputs = append(inputs, graphmodel.Node{Label: l})
	}

	got := transform.Describe(meta, inputs,
		map[string]string{"check": "dns", "API_KEY": "s3cret-value", "with_subdomains": "true"},
		map[string]string{"API_KEY": "MY_KEY"})
	want := "on 7 domains: a.test, b.test, c.test, d.test, e.test and 2 more. check=dns, API_KEY=secret MY_KEY, with_subdomains=true"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}

	if one := transform.Describe(meta, inputs[:1], nil, nil); one != "on 1 domain: a.test. API_KEY=secret API_KEY (not stored)" {
		t.Errorf("one input = %q", one)
	}
}

// A URL param is logged without what could carry a key: the user, the
// password, the query and the fragment.
func TestDescribeHidesWhatAURLCarries(t *testing.T) {
	meta := transform.Meta{Params: []transform.Param{{Name: "webhook_url", Type: transform.ParamURL}}}
	for in, want := range map[string]string{ //nolint:gosec // test URLs, no real credential
		"https://u:p@hooks.example.com/w/abc?token=t#f": "webhook_url=https://hooks.example.com/w/abc",
		"not a url": "webhook_url=(a URL)",
	} {
		if got := transform.DescribeParams(meta, map[string]string{"webhook_url": in}, nil); got != want {
			t.Errorf("%q logged as %q, want %q", in, got, want)
		}
	}
}

// A URL that does not parse is not echoed in the error, its query may
// hold a key.
func TestRedactURLErrorHidesAnUnparsableURL(t *testing.T) {
	_, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.example.com/?key=s3cret\n", nil)
	if err == nil {
		t.Fatal("a control character went through")
	}

	if got := transform.RedactURLError(err).Error(); strings.Contains(got, "s3cret") {
		t.Errorf("redacted = %q", got)
	}
}
