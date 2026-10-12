// Gosint, Copyright (c) 2026 YouSysAdmin

package release_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/release"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		tag, current string
		want         bool
	}{
		{"v1.2.4", "1.2.3", true},
		{"v1.10.0", "v1.9.9", true},
		{"v2.0.0", "1.99.99", true},
		{"v1.2.3", "1.2.3", false},
		{"v1.2.2", "1.2.3", false},
		{"v1.3.0-rc.1", "1.2.3", false},
		{"v1.2.3", "v1.2.3-4-gabcdef-dirty", false},
		{"v1.2.4", "v1.2.3-4-gabcdef", true},
		{"v1.2.4", "devel", false},
		{"nonsense", "1.2.3", false},
	}

	for _, c := range cases {
		if got := release.Newer(c.tag, c.current); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.tag, c.current, got, c.want)
		}
	}
}

func github(t *testing.T, body string, calls *atomic.Int32) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv
}

// A newer release is offered once and the answer is kept, GitHub is
// not asked on every page load.
func TestANewerReleaseIsOfferedAndKept(t *testing.T) {
	var calls atomic.Int32

	srv := github(t, `{"tag_name":"v1.3.0","html_url":"https://example.test/r/v1.3.0"}`, &calls)
	c := &release.Checker{Current: "v1.2.0", URL: srv.URL}

	st := c.Status(context.Background())
	if !st.Available || st.Latest != "1.3.0" || st.URL != "https://example.test/r/v1.3.0" {
		t.Fatalf("status = %+v", st)
	}

	c.Status(context.Background())

	if n := calls.Load(); n != 1 {
		t.Errorf("github asked %d times, want 1", n)
	}
}

func TestAPreReleaseIsNotOffered(t *testing.T) {
	var calls atomic.Int32

	srv := github(t, `{"tag_name":"v9.0.0","prerelease":true}`, &calls)
	c := &release.Checker{Current: "1.2.0", URL: srv.URL}

	if st := c.Status(context.Background()); st.Available {
		t.Errorf("a pre-release was offered: %+v", st)
	}
}

// A development build has no version to compare, nothing is sent.
func TestADevelopmentBuildAsksNothing(t *testing.T) {
	var calls atomic.Int32

	srv := github(t, `{"tag_name":"v9.0.0"}`, &calls)
	c := &release.Checker{Current: "devel", URL: srv.URL}

	if st := c.Status(context.Background()); st.Available || st.Current != "devel" {
		t.Errorf("status = %+v", st)
	}

	if calls.Load() != 0 {
		t.Error("a development build asked github")
	}
}
