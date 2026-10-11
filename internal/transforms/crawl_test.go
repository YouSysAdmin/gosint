// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/transforms/website"
)

func site(t *testing.T) string {
	t.Helper()

	pages := map[string]string{
		"/": `<html><body><a href="/contact">Contact</a><a href="https://github.com/acme">GitHub</a>
			<script>gtag('config','G-ABCDEF1234'); fbq('init', '123456789012345');</script></body></html>`,
		"/contact": `<html><body>Write to <a href="mailto:info@acme.io">info@acme.io</a> or call +33 6 12 34 56 78.
			<a href="https://twitter.com/acme">X</a><img src="logo@2x.png"></body></html>`,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

func TestCrawlers(t *testing.T) {
	base := site(t)

	links := newHarness(t)
	n := links.node("website", map[string]any{"url": "https://acme.io"})
	n.Properties["url"] = base + "/"
	links.run(website.WebsiteToLinks{}, map[string]string{"max_pages": "5", "delay_ms": "0"}, n)
	got := links.edges()
	for _, want := range []string{"https://acme.io/ -LINKS_TO-> https://github.com/", "https://acme.io/ -LINKS_TO-> https://twitter.com/"} {
		if !slices.Contains(got, want) {
			t.Errorf("links: missing %q in %q", want, got)
		}
	}

	contacts := newHarness(t)
	n = contacts.node("website", map[string]any{"url": "https://acme.io"})
	n.Properties["url"] = base + "/"
	contacts.run(website.WebsiteToCrawler{}, map[string]string{"max_pages": "5", "delay_ms": "0"}, n)
	contacts.expectEdges("https://acme.io/ -HAS_EMAIL-> info@acme.io", "https://acme.io/ -HAS_PHONE-> +33612345678")

	trackers := newHarness(t)
	n = trackers.node("website", map[string]any{"url": "https://acme.io"})
	n.Properties["url"] = base + "/"
	trackers.run(website.WebsiteToWebTrackers{}, nil, n)
	got = trackers.edges()
	if len(got) != 2 || !strings.Contains(strings.Join(got, " "), "G-ABCDEF1234") || !strings.Contains(strings.Join(got, " "), "123456789012345") {
		t.Errorf("trackers = %q", got)
	}
}

// The delay holds between two requests to the site and the page limit
// stops the crawl: two pages read at 300ms apart take at least that.
func TestCrawlWaitsBetweenRequests(t *testing.T) {
	var (
		mu   sync.Mutex
		hits []time.Time
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits = append(hits, time.Now())
		mu.Unlock()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<a href="/a">a</a><a href="/b">b</a><a href="/c">c</a>`))
	}))
	t.Cleanup(srv.Close)

	h := newHarness(t)
	n := h.node("website", map[string]any{"url": "https://acme.io"})
	n.Properties["url"] = srv.URL + "/"
	h.run(website.WebsiteToLinks{}, map[string]string{"max_pages": "2", "delay_ms": "300"}, n)

	mu.Lock()
	defer mu.Unlock()

	if len(hits) != 2 {
		t.Fatalf("%d requests, want the 2 the limit allows", len(hits))
	}

	if gap := hits[1].Sub(hits[0]); gap < 300*time.Millisecond {
		t.Errorf("requests %v apart, want at least 300ms", gap)
	}
}
