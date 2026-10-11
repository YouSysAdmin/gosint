// Gosint, Copyright (c) 2026 YouSysAdmin

// Package template runs transforms declared as YAML: one HTTP request per
// input entity, its answer mapped onto an output entity type.
//
// Requests go through the client that refuses internal addresses at
// connect time, so a template cannot reach what only the server can,
// whatever a DNS answer or a redirect says.
package template

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/yousysadmin/gosint/internal/core/types"
)

// Template is one transform declared as YAML.
type Template struct {
	Name        string   `yaml:"name"        json:"name"`
	Description string   `yaml:"description" json:"description"`
	Category    string   `yaml:"category"    json:"category"`
	Version     float64  `yaml:"version"     json:"version"`
	Input       Input    `yaml:"input"       json:"input"`
	Request     Request  `yaml:"request"     json:"request"`
	Response    Response `yaml:"response"    json:"response"`
	Output      Output   `yaml:"output"      json:"output"`
	Secrets     []Secret `yaml:"secrets"     json:"secrets"`
	Retry       *Retry   `yaml:"retry"       json:"retry"`
}

// Input is the entity type a template takes, and which of its
// properties fills the {{key}} placeholder. "nodeLabel" is the label.
type Input struct {
	Type string `yaml:"type" json:"type"`
	Key  string `yaml:"key"  json:"key"`
}

// Request is the HTTP request, with {{placeholders}}.
type Request struct {
	Method  string            `yaml:"method"  json:"method"`
	URL     string            `yaml:"url"     json:"url"`
	Headers map[string]string `yaml:"headers" json:"headers"`
	Params  map[string]string `yaml:"params"  json:"params"`
	Body    string            `yaml:"body"    json:"body"`
	Timeout float64           `yaml:"timeout" json:"timeout"`
}

// Response says how to read the answer: json, xml or text, and which
// output field each answer path fills (dot notation, numbers index
// arrays).
type Response struct {
	Expect string            `yaml:"expect" json:"expect"`
	Map    map[string]string `yaml:"map"    json:"map"`
}

// Output is the entity type a template produces. IsArray maps each item
// of the array at ArrayPath to one entity. Relation labels the edge
// from the input, RELATED_TO by default.
type Output struct {
	Type      string `yaml:"type"       json:"type"`
	IsArray   bool   `yaml:"is_array"   json:"is_array"`
	ArrayPath string `yaml:"array_path" json:"array_path"`
	Relation  string `yaml:"relation"   json:"relation"`
}

// Secret is a secret the template reads as {{secrets.NAME}}.
type Secret struct {
	Name        string `yaml:"name"        json:"name"`
	Required    *bool  `yaml:"required"    json:"required"`
	Description string `yaml:"description" json:"description"`
}

// IsRequired is the required flag, true unless set false.
func (s Secret) IsRequired() bool { return s.Required == nil || *s.Required }

// Retry is how a failed request is retried.
type Retry struct {
	MaxRetries    int     `yaml:"max_retries"     json:"max_retries"`
	BackoffFactor float64 `yaml:"backoff_factor"  json:"backoff_factor"`
	RetryOnStatus []int   `yaml:"retry_on_status" json:"retry_on_status"`
}

var (
	nameRe       = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)
	headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	secretNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	relationRe   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

// Parse reads and checks a template. Unknown keys are refused, so a
// misspelt one is an error rather than a silently ignored setting.
func Parse(source []byte, reg *types.Registry) (*Template, error) {
	dec := yaml.NewDecoder(bytes.NewReader(source))
	dec.KnownFields(true)

	var t Template
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("template: %w", err)
	}

	t.defaults()
	if err := t.check(reg); err != nil {
		return nil, err
	}

	return &t, nil
}

func (t *Template) defaults() {
	t.Name = strings.TrimSpace(t.Name)
	t.Input.Type = strings.ToLower(t.Input.Type)
	t.Output.Type = strings.ToLower(t.Output.Type)
	t.Input.Key = orDefault(t.Input.Key, "nodeLabel")
	t.Request.Method = strings.ToUpper(orDefault(t.Request.Method, "GET"))
	t.Response.Expect = orDefault(t.Response.Expect, "json")
	t.Output.Relation = orDefault(t.Output.Relation, "RELATED_TO")
	if t.Request.Timeout == 0 {
		t.Request.Timeout = 30
	}

	if t.Retry == nil {
		t.Retry = &Retry{MaxRetries: 3, BackoffFactor: 0.5}
	}

	if t.Retry.RetryOnStatus == nil {
		t.Retry.RetryOnStatus = []int{429, 500, 502, 503, 504}
	}
}

func (t *Template) check(reg *types.Registry) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if !nameRe.MatchString(t.Name) {
		fail("name must be lowercase letters, digits, dash and underscore")
	}

	if strings.TrimSpace(t.Category) == "" {
		fail("category is required")
	}

	for _, typ := range []string{t.Input.Type, t.Output.Type} {
		if _, ok := reg.Get(typ); !ok {
			fail("type %q is not a known entity type", typ)
		}
	}

	if t.Request.Method != "GET" && t.Request.Method != "POST" {
		fail("request.method must be GET or POST")
	}

	u, err := url.Parse(strings.ReplaceAll(t.Request.URL, "{{", "x"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		fail("request.url must be an http or https URL")
	}

	if t.Request.Timeout < 1 || t.Request.Timeout > 300 {
		fail("request.timeout must be between 1 and 300 seconds")
	}

	for name := range t.Request.Headers {
		if !headerNameRe.MatchString(name) {
			fail("request header %q is not a valid name", name)
		}
	}

	switch t.Response.Expect {
	case "json", "xml", "text":
	default:
		fail("response.expect must be json, xml or text")
	}

	if len(t.Response.Map) == 0 {
		fail("response.map must map at least one output field")
	}

	if !relationRe.MatchString(t.Output.Relation) {
		fail("output.relation must be UPPER_SNAKE_CASE")
	}

	for _, s := range t.Secrets {
		if !secretNameRe.MatchString(s.Name) {
			fail("secret %q is not a valid secret name", s.Name)
		}
	}

	r := t.Retry
	if r.MaxRetries < 0 || r.MaxRetries > 10 || r.BackoffFactor < 0.1 || r.BackoffFactor > 10 {
		fail("retry needs max_retries between 0 and 10 and backoff_factor between 0.1 and 10")
	}

	return errors.Join(errs...)
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}

	return strings.TrimSpace(v)
}
