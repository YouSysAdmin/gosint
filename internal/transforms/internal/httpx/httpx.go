// Gosint, Copyright (c) 2026 YouSysAdmin

// Package httpx is how transforms talk HTTP: JSON APIs through the plain
// client, pages of a target through the client refusing internal
// addresses, every request bounded by a timeout.
package httpx

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json/v2"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// MaxBody caps what a JSON API answer may weigh, MaxPage how much of a
// page is read.
const (
	MaxBody = 32 << 20
	MaxPage = 4 << 20
)

// RequestTimeout bounds one API call, SlowTimeout one to a source known
// to be slow.
const (
	RequestTimeout = 30 * time.Second
	SlowTimeout    = 90 * time.Second
)

// StatusError is an API answering anything but 200 or 404.
type StatusError struct {
	Host, Status string
	Code         int
}

func (e StatusError) Error() string {
	return e.Host + " answered " + e.Status
}

// GetJSON fetches url into out within RequestTimeout, through the
// client for fixed APIs. found is false on a 404, which most of these
// APIs answer for "nothing known".
func GetJSON(ctx context.Context, x *transform.Exec, url string, header http.Header, out any) (found bool, err error) {
	return getJSON(ctx, x.HTTP, RequestTimeout, url, header, out)
}

// GetJSONWithin is GetJSON with its own timeout.
func GetJSONWithin(ctx context.Context, timeout time.Duration, x *transform.Exec, url string, header http.Header, out any) (bool, error) {
	return getJSON(ctx, x.HTTP, timeout, url, header, out)
}

// GetJSONWeb is GetJSON through the client that refuses internal
// addresses, for an API whose URL a parameter names: a person able to
// launch a transform must not point the server at its own network.
func GetJSONWeb(ctx context.Context, x *transform.Exec, url string, header http.Header, out any) (bool, error) {
	return getJSON(ctx, x.Web, RequestTimeout, url, header, out)
}

// PostJSON posts body as JSON to url and reads the answer into out,
// within RequestTimeout, through the client for fixed APIs. found is
// false on a 404.
func PostJSON(ctx context.Context, x *transform.Exec, url string, header http.Header, body, out any) (bool, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return false, err
	}

	h := header.Clone()
	if h == nil {
		h = http.Header{}
	}

	h.Set("Content-Type", "application/json")

	return doJSON(ctx, x.HTTP, RequestTimeout, http.MethodPost, url, h, raw, out)
}

func getJSON(ctx context.Context, client *http.Client, timeout time.Duration, url string, header http.Header, out any) (bool, error) {
	return doJSON(ctx, client, timeout, http.MethodGet, url, header, nil, out)
}

func doJSON(ctx context.Context, client *http.Client, timeout time.Duration, method, url string, header http.Header, body []byte, out any) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return false, transform.RedactURLError(err)
	}

	maps.Copy(req.Header, header)

	resp, err := client.Do(req)
	if err != nil {
		return false, transform.RedactURLError(err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode != http.StatusOK {
		return false, StatusError{Host: req.URL.Host, Status: resp.Status, Code: resp.StatusCode}
	}

	answer, err := io.ReadAll(io.LimitReader(resp.Body, MaxBody))
	if err != nil {
		return false, err
	}

	if err := json.Unmarshal(answer, out); err != nil {
		return false, fmt.Errorf("%s answered unreadable JSON: %w", req.URL.Host, err)
	}

	return true, nil
}

// Head reports whether url answers 200 to a HEAD.
func Head(ctx context.Context, x *transform.Exec, url string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, transform.RedactURLError(err)
	}

	resp, err := x.HTTP.Do(req)
	if err != nil {
		return false, transform.RedactURLError(err)
	}

	_ = resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Probe sends a HEAD to url through the client that refuses internal
// addresses, for a site an entity names, and reports the status.
func Probe(ctx context.Context, x *transform.Exec, url string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, transform.RedactURLError(err)
	}

	resp, err := x.Web.Do(req)
	if err != nil {
		return 0, transform.RedactURLError(err)
	}

	_ = resp.Body.Close()

	return resp.StatusCode, nil
}

// Fetched is one page as the server answered it.
type Fetched struct {
	URL        string
	Status     int
	Header     http.Header
	Body       []byte
	Redirects  []string
	TLS        *tls.ConnectionState
	ReachedURL string
}

// Fetch GETs url through the client that refuses internal addresses,
// following redirects and recording them.
func Fetch(ctx context.Context, x *transform.Exec, url string) (*Fetched, error) {
	ctx, cancel := context.WithTimeout(ctx, RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, transform.RedactURLError(err)
	}

	resp, err := x.Web.Do(req)
	if err != nil {
		return nil, transform.RedactURLError(err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPage))
	if err != nil {
		return nil, err
	}

	f := &Fetched{URL: url, Status: resp.StatusCode, Header: resp.Header, Body: body, TLS: resp.TLS,
		ReachedURL: resp.Request.URL.String()}
	for r := resp.Request; r.Response != nil; r = r.Response.Request {
		f.Redirects = append(f.Redirects, r.Response.Request.URL.String())
	}

	slices.Reverse(f.Redirects)

	return f, nil
}
