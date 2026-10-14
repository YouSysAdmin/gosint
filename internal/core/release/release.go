// Gosint, Copyright (c) 2026 YouSysAdmin

// Package release tells whether GitHub has a newer stable release than
// the running build. Pre-releases and drafts are never offered.
package release

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// LatestURL is GitHub's newest release that is neither a draft nor
	// a pre-release.
	LatestURL = "https://api.github.com/repos/YouSysAdmin/gosint/releases/latest"

	// DefaultEvery is how long an answer is kept before GitHub is asked
	// again.
	DefaultEvery = 12 * time.Hour

	// retryAfter is how soon a failed check is tried again.
	retryAfter = time.Hour

	// requestTimeout bounds one request to GitHub.
	requestTimeout = 10 * time.Second

	// maxBody bounds the release document read.
	maxBody = 1 << 20
)

// Status is what the console is told.
type Status struct {
	Current   string    `json:"current"`
	Latest    string    `json:"latest,omitempty"`
	URL       string    `json:"url,omitempty"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checked_at,omitzero"`
}

// Checker asks GitHub at most once per Every, only when someone asks it.
type Checker struct {
	// Current is the running version, a build that is not a release
	// (devel, a bare commit) is never checked.
	Current string

	// URL and Client are GitHub's endpoint and the client reaching it,
	// set by tests.
	URL    string
	Client *http.Client
	Agent  string
	Every  time.Duration

	mu       sync.Mutex
	status   Status
	next     time.Time
	inflight bool
}

// release is the part of GitHub's answer read.
type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// Status answers the last check, checking again once it is due. A
// failure keeps the previous answer. One request is out at a time, a
// caller arriving while it is on its way gets the answer kept so far
// instead of waiting on GitHub.
func (c *Checker) Status(ctx context.Context) Status {
	if _, ok := parse(c.Current); !ok {
		return Status{Current: c.Current}
	}

	c.mu.Lock()
	now := time.Now().UTC()
	if now.Before(c.next) || c.inflight {
		st := c.kept()
		c.mu.Unlock()

		return st
	}

	c.inflight = true
	c.mu.Unlock()

	// A closed tab must not cut the check every other admin waits on.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), requestTimeout)
	defer cancel()

	rel, err := c.fetch(ctx)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.inflight = false
	if err != nil {
		c.next = now.Add(retryAfter)

		return c.kept()
	}

	c.status = Status{Current: c.Current, CheckedAt: now}
	if !rel.Draft && !rel.Prerelease && Newer(rel.TagName, c.Current) {
		c.status.Latest = strings.TrimPrefix(rel.TagName, "v")
		c.status.URL = rel.HTMLURL
		c.status.Available = true
	}

	every := c.Every
	if every <= 0 {
		every = DefaultEvery
	}

	c.next = now.Add(every)

	return c.status
}

// kept is the answer so far, the running version alone before the
// first one. Called under mu.
func (c *Checker) kept() Status {
	if c.status.Current == "" {
		return Status{Current: c.Current}
	}

	return c.status
}

func (c *Checker) fetch(ctx context.Context) (release, error) {
	url := c.URL
	if url == "" {
		url = LatestURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return release{}, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Agent != "" {
		req.Header.Set("User-Agent", c.Agent)
	}

	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return release{}, err
	}

	defer resp.Body.Close() //nolint:errcheck // read only

	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("github answered %d", resp.StatusCode)
	}

	var rel release
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, maxBody), &rel); err != nil {
		return release{}, fmt.Errorf("release document: %w", err)
	}

	return rel, nil
}

// Newer tells whether tag is a stable release above current. A tag
// with a pre-release part is never newer.
func Newer(tag, current string) bool {
	t, ok := parse(tag)
	if !ok || strings.Contains(strings.TrimPrefix(tag, "v"), "-") {
		return false
	}

	c, ok := parse(current)
	if !ok {
		return false
	}

	for i := range t {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}

	return false
}

// parse reads MAJOR.MINOR.PATCH, a suffix after a dash ignored, so a
// build described by git (v1.2.3-4-gabc) counts as 1.2.3.
func parse(v string) ([3]int, bool) {
	var out [3]int

	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	v, _, _ = strings.Cut(v, "+")

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}

	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}

		out[i] = n
	}

	return out, true
}
