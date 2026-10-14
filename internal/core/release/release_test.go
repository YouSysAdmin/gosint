// Gosint, Copyright (c) 2026 YouSysAdmin

package release_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

// GitHub being down keeps the last answer, and is not asked again at
// once.
func TestAFailedCheckKeepsTheLastAnswer(t *testing.T) {
	var calls atomic.Int32

	var down atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		if down.Load() {
			w.WriteHeader(http.StatusBadGateway)

			return
		}

		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0"}`))
	}))
	t.Cleanup(srv.Close)

	c := &release.Checker{Current: "1.2.0", URL: srv.URL, Every: time.Nanosecond}
	if st := c.Status(context.Background()); !st.Available {
		t.Fatalf("status = %+v", st)
	}

	down.Store(true)

	if st := c.Status(context.Background()); !st.Available || st.Latest != "1.3.0" {
		t.Fatalf("a failure dropped the answer: %+v", st)
	}

	c.Status(context.Background())

	if n := calls.Load(); n != 2 {
		t.Errorf("github asked %d times, want 2: a failure is retried after an hour", n)
	}
}

// A check on its way is not run twice: a second caller gets the answer
// kept so far at once instead of waiting on GitHub.
func TestOneRequestAtATime(t *testing.T) {
	var calls atomic.Int32

	entered := make(chan struct{}, 2)
	proceed := make(chan struct{})

	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(proceed) }) })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		<-proceed
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0"}`))
	}))
	t.Cleanup(srv.Close)

	c := &release.Checker{Current: "1.2.0", URL: srv.URL}

	first := make(chan release.Status, 1)
	go func() { first <- c.Status(context.Background()) }()

	<-entered

	second := make(chan release.Status, 1)
	go func() { second <- c.Status(context.Background()) }()

	select {
	case st := <-second:
		if st.Available || st.Current != "1.2.0" {
			t.Fatalf("second status = %+v, want the version alone", st)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the second caller waited on the first's request")
	}

	once.Do(func() { close(proceed) })

	if st := <-first; !st.Available {
		t.Fatalf("first status = %+v", st)
	}

	if n := calls.Load(); n != 1 {
		t.Errorf("github asked %d times, want 1", n)
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
