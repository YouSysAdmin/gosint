// Gosint, Copyright (c) 2026 YouSysAdmin

package template_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/template"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
)

type discard struct{}

func (discard) Log(logmodel.Level, string, map[string]any) {}

func parse(t *testing.T, yaml string) *template.Template {
	t.Helper()

	tpl, err := template.Parse([]byte(yaml), types.Builtin())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	return tpl
}

func exec(params map[string]string) *transform.Exec {
	return transform.NewExec(transform.ExecConfig{Log: discard{}, Params: params, HTTP: http.DefaultClient, Web: http.DefaultClient, Types: types.Builtin()})
}

const githubLike = `
name: github-user
category: Username
version: 1.0
input:
  type: Username
  key: value
secrets:
  - name: GITHUB_TOKEN
    required: false
request:
  method: GET
  url: "%s/users/{{value}}"
  headers:
    Authorization: "Bearer {{secrets.GITHUB_TOKEN}}"
response:
  expect: json
  map:
    username: login
    display_name: name
    platform: site.name
output:
  type: SocialAccount
  relation: HAS_SOCIAL_ACCOUNT
`

func TestATemplateMapsTheAnswer(t *testing.T) {
	var auth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization") + " " + r.URL.EscapedPath())
		_, _ = w.Write([]byte(`{"login":"octo","name":"Octo Cat","site":{"name":"github"}}`))
	}))
	defer srv.Close()

	tpl := parse(t, strings.Replace(githubLike, "%s", srv.URL, 1))
	in := graphmodel.Node{Type: "username", Label: "octo/../admin", Properties: map[string]any{"value": "octo/../admin"}}

	outs, answer, err := template.Transform{T: tpl}.Execute(t.Context(), exec(map[string]string{"GITHUB_TOKEN": "tok"}), in)
	if err != nil || answer.Status != 200 {
		t.Fatalf("execute = %v, %+v", err, answer)
	}

	if got := auth.Load().(string); got != "Bearer tok /users/octo%2F..%2Fadmin" {
		t.Errorf("request = %q, want the input escaped as one path segment", got)
	}

	if len(outs) != 1 || outs[0]["username"] != "octo" || outs[0]["platform"] != "github" {
		t.Errorf("mapped = %+v", outs)
	}
}

func TestArrayAnswersAndRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		_, _ = w.Write([]byte(`{"data":{"hosts":[{"h":"a.example.com"},{"h":"b.example.com"}]}}`))
	}))
	defer srv.Close()

	tpl := parse(t, `
name: hosts
category: Domain
version: 1
input: {type: domain, key: domain}
request: {url: "`+srv.URL+`/{{domain}}"}
response: {map: {domain: h}}
output: {type: domain, is_array: true, array_path: data.hosts}
retry: {max_retries: 2, backoff_factor: 0.1}
`)

	outs, _, err := template.Transform{T: tpl}.Execute(t.Context(), exec(nil),
		graphmodel.Node{Type: "domain", Label: "example.com", Properties: map[string]any{"domain": "example.com"}})
	if err != nil || len(outs) != 2 || outs[1]["domain"] != "b.example.com" || calls.Load() != 2 {
		t.Errorf("outs = %+v, err %v, calls %d", outs, err, calls.Load())
	}
}

func TestXMLAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<r><geo><city>Paris</city><country>FR</country></geo></r>`))
	}))
	defer srv.Close()

	tpl := parse(t, `
name: geo
category: Ip
version: 1
input: {type: ip, key: address}
request: {url: "`+srv.URL+`/{{address}}"}
response: {expect: xml, map: {city: geo.city, country: country}}
output: {type: ip}
`)

	outs, _, err := template.Transform{T: tpl}.Execute(t.Context(), exec(nil),
		graphmodel.Node{Type: "ip", Properties: map[string]any{"address": "8.8.8.8"}})
	if err != nil || outs[0]["city"] != "Paris" || outs[0]["country"] != "FR" {
		t.Errorf("outs = %+v, %v", outs, err)
	}
}

func TestParseRefusesBadTemplates(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"unknown key", "name: a\ncategory: c\nversion: 1\ninput: {type: ip}\nrequest: {url: 'https://x.io'}\nresponse: {map: {a: b}}\noutput: {type: ip}\nbogus: 1\n", "bogus"},
		{"unknown type", "name: a\ncategory: c\nversion: 1\ninput: {type: nosuch}\nrequest: {url: 'https://x.io'}\nresponse: {map: {a: b}}\noutput: {type: ip}\n", `"nosuch" is not a known entity type`},
		{"bad method", "name: a\ncategory: c\nversion: 1\ninput: {type: ip}\nrequest: {method: DELETE, url: 'https://x.io'}\nresponse: {map: {a: b}}\noutput: {type: ip}\n", "request.method must be GET or POST"},
		{"file scheme", "name: a\ncategory: c\nversion: 1\ninput: {type: ip}\nrequest: {url: 'file:///etc/passwd'}\nresponse: {map: {a: b}}\noutput: {type: ip}\n", "request.url must be an http or https URL"},
		{"empty mapping", "name: a\ncategory: c\nversion: 1\ninput: {type: ip}\nrequest: {url: 'https://x.io'}\nresponse: {map: {}}\noutput: {type: ip}\n", "response.map must map at least one output field"},
	} {
		_, err := template.Parse([]byte(tc.yaml), types.Builtin())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to say %q", tc.name, err, tc.want)
		}
	}
}

func TestAMissingPlaceholderIsAnError(t *testing.T) {
	tpl := parse(t, `
name: needs-key
category: Ip
version: 1
input: {type: ip, key: address}
secrets: [{name: API_KEY}]
request: {url: "https://api.example.com/{{address}}?k={{secrets.API_KEY}}"}
response: {map: {city: city}}
output: {type: ip}
`)

	_, _, err := template.Transform{T: tpl}.Execute(t.Context(), exec(nil),
		graphmodel.Node{Type: "ip", Properties: map[string]any{"address": "8.8.8.8"}})
	if err == nil || !strings.Contains(err.Error(), "secrets.API_KEY") {
		t.Errorf("err = %v", err)
	}
}

// A retry wait doubles from the factor and stops at a minute, so a
// template answering 503 cannot hold a worker for hours.
func TestRetryWaitsAreCapped(t *testing.T) {
	for _, tc := range []struct {
		factor  float64
		attempt int
		want    time.Duration
	}{
		{0.5, 1, 500 * time.Millisecond},
		{0.5, 3, 2 * time.Second},
		{10, 3, 40 * time.Second},
		{10, 4, template.MaxRetryWait},
		{10, 10, template.MaxRetryWait},
	} {
		if got := template.RetryWait(tc.factor, tc.attempt); got != tc.want {
			t.Errorf("RetryWait(%v, %d) = %v, want %v", tc.factor, tc.attempt, got, tc.want)
		}
	}

	// NoRetry sends once whatever the template says.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	tpl := parse(t, `
name: flaky
category: Domain
version: 1
input: {type: domain, key: domain}
request: {url: "`+srv.URL+`/{{domain}}"}
response: {map: {domain: h}}
output: {type: domain}
retry: {max_retries: 3, backoff_factor: 0.1}
`)

	_, answer, err := template.Transform{T: tpl, NoRetry: true}.Execute(t.Context(), exec(nil),
		graphmodel.Node{Type: "domain", Label: "example.com", Properties: map[string]any{"domain": "example.com"}})
	if err == nil || answer.Status != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Errorf("no retry: err %v, status %d, calls %d", err, answer.Status, calls.Load())
	}
}

// A body is escaped for its kind: a form value cannot add a field, an
// XML value cannot open an element.
func TestABodyIsEscapedForItsKind(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.Store(string(b))
		_, _ = w.Write([]byte(`{"login": "x"}`))
	}))
	defer srv.Close()

	label := "a&admin=1<x>"
	for _, c := range []struct{ body, headers, want string }{
		{`q={{ value }}&n=1`, ``, `q=a%26admin%3D1%3Cx%3E&n=1`},
		{`<q>{{value}}</q>`, ``, `<q>a&amp;admin=1&lt;x&gt;</q>`},
		{`{{value}}`, `{Content-Type: application/x-www-form-urlencoded}`, `a%26admin%3D1%3Cx%3E`},
		{`{"q": "{{value}}"}`, ``, `{"q": "a&admin=1<x>"}`},
	} {
		headers := ""
		if c.headers != "" {
			headers = ", headers: " + c.headers
		}

		tpl := parse(t, `
name: body
category: Username
version: 1
input: {type: username, key: value}
request: {method: POST, url: "`+srv.URL+`/", body: '`+c.body+`'`+headers+`}
response: {expect: json, map: {value: login}}
output: {type: username}
`)

		if _, _, err := (template.Transform{T: tpl}).Execute(t.Context(), exec(nil),
			graphmodel.Node{Type: "username", Properties: map[string]any{"value": label}}); err != nil {
			t.Fatal(err)
		}

		if s := got.Load().(string); s != c.want {
			t.Errorf("body %q sent %q, want %q", c.body, s, c.want)
		}
	}
}

// The template's own timeout bounds a request, not the client's: a
// slower API it allows for is waited for.
func TestTheTemplateTimeoutIsHonoured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"login": "x"}`))
	}))
	defer srv.Close()

	tpl := parse(t, `
name: slow
category: Username
version: 1
input: {type: username, key: value}
request: {url: "`+srv.URL+`/{{value}}", timeout: 5}
response: {expect: json, map: {value: login}}
output: {type: username}
`)

	short := &http.Client{Timeout: 50 * time.Millisecond}
	x := transform.NewExec(transform.ExecConfig{Log: discard{}, HTTP: short, Web: short, Types: types.Builtin()})
	if _, _, err := (template.Transform{T: tpl}).Execute(t.Context(), x,
		graphmodel.Node{Type: "username", Properties: map[string]any{"value": "a"}}); err != nil {
		t.Errorf("the client's timeout cut the request: %v", err)
	}
}
