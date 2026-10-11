// Gosint, Copyright (c) 2026 YouSysAdmin

package template

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// maxAnswer caps what one templated request may answer.
const maxAnswer = 16 << 20

// The retry waits are bounded, so a template answering 503 cannot hold
// a worker for hours: one wait and the sum of them.
const (
	MaxRetryWait   = time.Minute
	MaxRetryBudget = 5 * time.Minute
)

// Transform runs a template as a transform.Transform.
type Transform struct {
	T *Template

	// ID is the stored template's id, carried in Meta for the console.
	ID string

	// NoRetry sends each request once, for the editor's test run.
	NoRetry bool
}

// RetryWait is the wait before retry attempt (from 1): the backoff
// factor in seconds doubled each time, at most MaxRetryWait.
func RetryWait(factor float64, attempt int) time.Duration {
	wait := time.Duration(factor * float64(time.Second) * float64(int(1)<<(attempt-1)))

	return min(wait, MaxRetryWait)
}

// Meta describes the template as a transform. Its secrets are its
// params, resolved from the secrets like any transform's.
func (e Transform) Meta() transform.Meta {
	params := make([]transform.Param, 0, len(e.T.Secrets))
	for _, s := range e.T.Secrets {
		params = append(params, transform.Param{Name: s.Name, Type: transform.ParamSecret, Required: s.IsRequired(),
			Description: s.Description})
	}

	return transform.Meta{Name: e.T.Name, Category: e.T.Category, Description: e.T.Description,
		Input: e.T.Input.Type, Output: e.T.Output.Type, Params: params}
}

// Run sends the request for each input and links what it maps to.
// Every output links to its own input.
func (e Transform) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		outs, _, err := e.Execute(ctx, x, in)
		if err != nil {
			x.Warn("%s: %s: %v", e.T.Name, in.Label, err)

			continue
		}

		for _, props := range outs {
			if e.InPlace() {
				if _, err := x.Node(ctx, in.Type, e.Complete(in, props)); err != nil {
					x.Warn("%s: %s: %v", e.T.Name, in.Label, err)
				} else {
					x.Found("%s: enriched %s", e.T.Name, in.Label)
				}

				continue
			}

			if _, err := x.NodeAndLink(ctx, in, e.T.Output.Relation, e.T.Output.Type, props); err != nil {
				x.Warn("%s: %s: %v", e.T.Name, in.Label, err)
			}
		}
	}

	return nil
}

// InPlace reports whether the template enriches its input rather than
// finding other entities: same input and output type, no array.
func (e Transform) InPlace() bool {
	return e.T.Input.Type == e.T.Output.Type && !e.T.Output.IsArray
}

// Complete is what an in-place template writes: the input's properties
// with the mapped ones laid over, so the answer need not repeat the
// input's own primary field.
func (e Transform) Complete(in graphmodel.Node, mapped map[string]any) map[string]any {
	out := maps.Clone(in.Properties)
	if out == nil {
		out = map[string]any{}
	}

	maps.Copy(out, mapped)

	return out
}

// Answer is the raw HTTP answer, for the test endpoint.
type Answer struct {
	Status int    `json:"status_code"`
	Body   string `json:"body"`
}

// Execute sends the request for one input and returns the mapped
// output properties, unvalidated, plus the raw answer.
func (e Transform) Execute(ctx context.Context, x *transform.Exec, in graphmodel.Node) ([]map[string]any, *Answer, error) {
	values, err := e.values(x, in)
	if err != nil {
		return nil, nil, err
	}

	req, err := e.request(ctx, values)
	if err != nil {
		return nil, nil, err
	}

	status, body, err := e.send(ctx, x, req)
	answer := &Answer{Status: status, Body: string(body)}
	if err != nil {
		return nil, answer, err
	}

	outs, err := e.mapAnswer(body)

	return outs, answer, err
}

// values are the placeholders: the input key and the opened secrets.
func (e Transform) values(x *transform.Exec, in graphmodel.Node) (map[string]string, error) {
	key := e.T.Input.Key
	values := map[string]string{}

	switch key {
	case "nodeLabel", "label":
		values[key] = in.Label
	default:
		v, ok := in.Properties[key]
		if !ok || v == nil {
			return nil, fmt.Errorf("the input has no %s", key)
		}

		values[key] = fmt.Sprint(v)
	}

	for _, s := range e.T.Secrets {
		if v := x.Param(s.Name); v != "" {
			values["secrets."+s.Name] = v
		}
	}

	return values, nil
}

func (e Transform) request(ctx context.Context, values map[string]string) (*http.Request, error) {
	r := e.T.Request
	rawURL, err := render(r.URL, values, escapeURL)
	if err != nil {
		return nil, err
	}

	// The rendered URL may hold a secret, so a refusal names the host
	// at most.
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		if err == nil && u.Host != "" {
			return nil, fmt.Errorf("template: the request URL to %s is not http or https", u.Host)
		}

		return nil, errors.New("template: the request URL is not an http or https URL")
	}

	q := u.Query()
	for k, v := range r.Params {
		rendered, err := render(v, values, escapeNone)
		if err != nil {
			return nil, err
		}

		q.Set(k, rendered)
	}

	u.RawQuery = q.Encode()

	// A body gets its values escaped for its kind, so a label another
	// transform read off the network cannot reshape the request.
	body := ""
	if r.Body != "" {
		if body, err = render(r.Body, values, bodyEscape(r.Body, r.Headers)); err != nil {
			return nil, err
		}
	}

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), reader)
	if err != nil {
		return nil, transform.RedactURLError(err)
	}

	for k, v := range r.Headers {
		rendered, err := render(v, values, escapeNone)
		if err != nil {
			return nil, err
		}

		req.Header.Set(k, rendered)
	}

	return req, nil
}

// bodyEscape picks how placeholders are written into a body: by its
// declared Content-Type when there is one, else by its shape. A JSON
// body escapes as a JSON string, a form as a query value, XML or HTML
// as text, anything else as it is.
func bodyEscape(body string, headers map[string]string) escapeMode {
	for k, v := range headers {
		if !strings.EqualFold(k, "Content-Type") {
			continue
		}

		ct, _, _ := mime.ParseMediaType(v)
		switch {
		case ct == "application/json" || strings.HasSuffix(ct, "+json"):
			return escapeJSON
		case ct == "application/x-www-form-urlencoded":
			return escapeForm
		case strings.HasSuffix(ct, "xml") || ct == "text/html":
			return escapeXML
		}
	}

	t := strings.TrimSpace(body)
	switch {
	case strings.HasPrefix(t, "{") || strings.HasPrefix(t, "["):
		return escapeJSON
	case strings.HasPrefix(t, "<"):
		return escapeXML
	case formRe.MatchString(placeholderRe.ReplaceAllString(t, "x")):
		return escapeForm
	}

	return escapeNone
}

// formRe is a body shaped like name=value pairs.
var formRe = regexp.MustCompile(`^[A-Za-z0-9_.\-\[\]]+=[^\s]*(&[A-Za-z0-9_.\-\[\]]+=[^\s]*)*$`)

// send makes the request with the template's retries and backoff.
func (e Transform) send(ctx context.Context, x *transform.Exec, req *http.Request) (int, []byte, error) {
	retry := e.T.Retry
	timeout := time.Duration(e.T.Request.Timeout * float64(time.Second))

	retries := retry.MaxRetries
	if e.NoRetry {
		retries = 0
	}

	var (
		lastErr    error
		lastStatus int
		lastBody   []byte
		waited     time.Duration
	)

	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			wait := RetryWait(retry.BackoffFactor, attempt)
			if waited+wait > MaxRetryBudget {
				break
			}

			waited += wait
			t := time.NewTimer(wait)
			select {
			case <-t.C:
			case <-ctx.Done():
				t.Stop()

				return 0, nil, ctx.Err()
			}
		}

		status, body, err := e.once(ctx, x, req, timeout)
		if err == nil && !slices.Contains(retry.RetryOnStatus, status) {
			if status >= 400 {
				return status, body, fmt.Errorf("the API answered %d", status)
			}

			return status, body, nil
		}

		lastErr, lastStatus, lastBody = err, status, body
		if err == nil {
			lastErr = fmt.Errorf("the API answered %d", status)
		}
	}

	// The last answer travels with the error, so the test endpoint can
	// show what the API said.
	return lastStatus, lastBody, lastErr
}

func (e Transform) once(ctx context.Context, x *transform.Exec, req *http.Request, timeout time.Duration) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	r := req.Clone(ctx)
	if req.GetBody != nil {
		r.Body, _ = req.GetBody()
	}

	// The template's own timeout bounds the request through ctx, so the
	// client's ceiling (a minute) does not cut a longer one it declared.
	client := *x.Web
	client.Timeout = 0

	resp, err := client.Do(r) //nolint:gosec // Web refuses internal addresses at connect time
	if err != nil {
		return 0, nil, transform.RedactURLError(err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))

	return resp.StatusCode, body, err
}

// mapAnswer turns the answer into output properties, one set per array
// item when the output is an array.
func (e Transform) mapAnswer(body []byte) ([]map[string]any, error) {
	if e.T.Response.Expect == "xml" {
		root, err := parseXML(body)
		if err != nil {
			return nil, err
		}

		props := map[string]any{}
		for field, path := range e.T.Response.Map {
			if v, ok := root.find(path); ok {
				props[field] = v
			}
		}

		return []map[string]any{props}, nil
	}

	var data any
	if e.T.Response.Expect == "json" {
		if err := json.Unmarshal(body, &data); err != nil {
			return nil, fmt.Errorf("template: the answer is not JSON: %w", err)
		}
	} else {
		data = string(body)
	}

	items := []any{data}
	if e.T.Output.IsArray {
		list, ok := lookup(data, e.T.Output.ArrayPath).([]any)
		if !ok {
			return nil, errNotArray
		}

		items = list
	}

	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		props := map[string]any{}
		for field, path := range e.T.Response.Map {
			if v := lookup(item, path); v != nil {
				props[field] = v
			}
		}

		out = append(out, props)
	}

	return out, nil
}
