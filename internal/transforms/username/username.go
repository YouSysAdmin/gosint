// Gosint, Copyright (c) 2026 YouSysAdmin

// Package username holds the transforms that start from a username.
package username

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/hudsonrock"
)

// probes is how many sites are asked at once per username.
const probes = 8

// maxProbeBody bounds what is read of a profile page to look for the
// "no such user" sentence.
const maxProbeBody = 256 << 10

// UsernameToAccounts finds the sites a username has a profile on. It
// stands in for sherlock and maigret, with a list of its own: every
// site is asked directly through the guarded client, nothing is
// shelled out.
type UsernameToAccounts struct {
	Sites []Site
}

// Meta describes the transform.
func (e UsernameToAccounts) Meta() transform.Meta {
	return transform.Meta{Name: "username_to_accounts", Category: "Social", Input: "username", Output: "socialaccount",
		Description: "Find the sites a username has a profile on, by asking each one (GitHub, YouTube, Steam and more).",
		Params: []transform.Param{{Name: "sites", Type: transform.ParamString,
			Description: "Only these sites, comma-separated names such as GitHub, YouTube. Empty asks every site."}}}
}

// probe is one site's answer about one username.
type probe struct {
	site   Site
	url    string
	exists bool
	err    error
}

// Run enriches each username.
func (e UsernameToAccounts) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	sites := e.chosen(x.Param("sites"))
	if len(sites) == 0 {
		x.Warn("no site matches the sites parameter")

		return nil
	}

	for _, in := range inputs {
		name := strings.TrimPrefix(props.Str(in.Properties, "value"), "@")
		if name == "" {
			continue
		}

		found, unknown := 0, 0
		for _, p := range e.ask(ctx, x, sites, name) {
			if p.err != nil {
				unknown++
				x.Warn("%s: %s: %v", name, p.site.Name, p.err)

				continue
			}

			if !p.exists {
				continue
			}

			account := map[string]any{"username": map[string]any{"value": name}, "platform": p.site.Name, "profile_url": p.url}
			if _, err := x.NodeAndLink(ctx, in, "HAS_SOCIAL_ACCOUNT", "socialaccount", account); err != nil {
				x.Warn("%s: %s: %v", name, p.site.Name, err)

				continue
			}

			found++
		}

		x.Info("%s: %d profiles on %d sites, %d sites did not answer", name, found, len(sites), unknown)
	}

	return nil
}

// chosen is the sites the parameter names, or all of them.
func (e UsernameToAccounts) chosen(param string) []Site {
	if strings.TrimSpace(param) == "" {
		return e.Sites
	}

	want := map[string]bool{}
	for n := range strings.SplitSeq(param, ",") {
		want[strings.ToLower(strings.TrimSpace(n))] = true
	}

	var out []Site
	for _, s := range e.Sites {
		if want[strings.ToLower(s.Name)] {
			out = append(out, s)
		}
	}

	return out
}

// ask probes every site for name, a few at a time. The answers come
// back in site order, the graph is written by the caller on one
// goroutine.
func (e UsernameToAccounts) ask(ctx context.Context, x *transform.Exec, sites []Site, name string) []probe {
	out := make([]probe, len(sites))
	slots := make(chan struct{}, probes)

	var wg sync.WaitGroup
	for i, s := range sites {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()

			out[i] = check(ctx, x.Web, s, name)
		})
	}

	wg.Wait()

	return out
}

// browserAgent is what a profile request says it is unless the site
// is Plain. Wikipedia, Linktree, Chess.com and Behance refuse a
// server's agent outright (403, 406, 429) and answer a browser's
// honestly, 404 included.
const browserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

// profileURL puts name where the site's URL has {}, escaped for a query
// value after the question mark and for a path segment before it.
func profileURL(tmpl, name string) string {
	q, at := strings.Index(tmpl, "?"), strings.Index(tmpl, "{}")
	if q >= 0 && q < at {
		return strings.ReplaceAll(tmpl, "{}", url.QueryEscape(name))
	}

	return strings.ReplaceAll(tmpl, "{}", url.PathEscape(name))
}

// check asks one site. 200 is a profile, 404 (or the site's own absent
// status, or its absent sentence) is none, a bot wall or anything else
// is an unknown that is reported but not drawn.
func check(ctx context.Context, client *http.Client, s Site, name string) probe {
	p := probe{site: s, url: profileURL(s.URL, name)}

	ctx, cancel := context.WithTimeout(ctx, httpx.RequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		p.err = err

		return p
	}

	req.Header.Set("Accept", "text/html,application/json")
	if !s.Plain {
		req.Header.Set("User-Agent", browserAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		p.err = transform.RedactURLError(err)

		return p
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	switch {
	case resp.StatusCode == http.StatusNotFound, s.AbsentStatus != 0 && resp.StatusCode == s.AbsentStatus:
		return p
	case resp.StatusCode != http.StatusOK:
		p.err = httpx.StatusError{Host: req.URL.Host, Status: resp.Status, Code: resp.StatusCode}

		return p
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxProbeBody))
	for _, w := range walls {
		if strings.Contains(string(body), w) {
			p.err = errors.New(req.URL.Host + " answered a bot check instead of the page")

			return p
		}
	}

	if s.Absent != "" && strings.Contains(string(body), s.Absent) {
		return p
	}

	p.exists = true

	return p
}

// All is the package's transforms.
func All() []transform.Transform {
	return []transform.Transform{UsernameToAccounts{Sites: Builtin}, UsernameToInfostealers{Client: hudsonrock.Public}, UsernameToDehashed{Client: dehashed.Public}}
}
