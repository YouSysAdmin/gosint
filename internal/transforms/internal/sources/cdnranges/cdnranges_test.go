// Gosint, Copyright (c) 2026 YouSysAdmin

package cdnranges_test

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/cdnranges"
)

type lines []string

func (l *lines) Log(_ logmodel.Level, message string, _ map[string]any) { *l = append(*l, message) }

func exec(dir string) (*transform.Exec, *lines) {
	var log lines

	return transform.NewExec(transform.ExecConfig{HTTP: http.DefaultClient, Log: &log, CacheDir: dir}), &log
}

// feeds serves every provider's shape and counts the requests. A path
// in down answers 502.
func feeds(t *testing.T, down map[string]bool) (*httptest.Server, *int) {
	t.Helper()

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if down[r.URL.Path] {
			http.Error(w, "down", http.StatusBadGateway)

			return
		}

		switch r.URL.Path {
		case "/v4":
			_, _ = w.Write([]byte("# comment\n104.16.0.0/12\n\n"))
		case "/fastly":
			_, _ = w.Write([]byte(`{"addresses":["23.235.32.0/20"],"ipv6_addresses":["2a04:4e40::/32"]}`))
		case "/aws":
			_, _ = w.Write([]byte(`{"prefixes":[{"ip_prefix":"3.5.140.0/22"}],"ipv6_prefixes":[{"ipv6_prefix":"2600:1f00::/40"}]}`))
		case "/gcp":
			_, _ = w.Write([]byte(`{"prefixes":[{"ipv4Prefix":"34.80.0.0/15"},{"ipv6Prefix":"2600:1900::/35"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, &calls
}

func table(base string, now *time.Time) *cdnranges.Table {
	return &cdnranges.Table{TTL: 48 * time.Hour, Now: func() time.Time { return *now }, Providers: []cdnranges.Provider{
		{Name: "cloudflare", URLs: []string{base + "/v4"}, Parse: cdnranges.ParseLines},
		{Name: "fastly", URLs: []string{base + "/fastly"}, Parse: cdnranges.Fastly.Parse},
		{Name: "aws", URLs: []string{base + "/aws"}, Parse: cdnranges.AWS.Parse},
		{Name: "gcp", URLs: []string{base + "/gcp"}, Parse: cdnranges.GCP.Parse},
	}}
}

// Each feed's shape is read, and an unknown provider is refused.
func TestFeedsAreRead(t *testing.T) {
	srv, _ := feeds(t, nil)
	now := time.Now()
	tb := table(srv.URL, &now)
	x, _ := exec("")

	for name, want := range map[string]int{"cloudflare": 1, "fastly": 2, "aws": 2, "gcp": 2} {
		got, err := tb.Prefixes(t.Context(), x, name)
		if err != nil || len(got) != want {
			t.Errorf("%s = %v %v, want %d prefixes", name, got, err, want)
		}
	}

	got, _ := tb.Prefixes(t.Context(), x, "cloudflare")
	if !cdnranges.Contains(got, netip.MustParseAddr("104.16.1.1")) {
		t.Error("104.16.1.1 is not in 104.16.0.0/12")
	}

	if _, err := tb.Prefixes(t.Context(), x, "akamai"); err == nil {
		t.Error("an unknown provider passed")
	}
}

// The first use downloads into the cache directory, a later process
// reads the file while it is younger than the TTL, an older file is
// fetched again, and a feed that is down falls back to the old file with
// a warning. With no file at all a dead feed is an error.
func TestRangesAreCachedOnDiskByTheFileDate(t *testing.T) {
	down := map[string]bool{}
	srv, calls := feeds(t, down)
	dir := t.TempDir()
	file := filepath.Join(dir, "cdnranges", "cloudflare.txt")
	now := time.Now()
	x, log := exec(dir)

	if _, err := table(srv.URL, &now).Prefixes(t.Context(), x, "cloudflare"); err != nil || *calls != 1 {
		t.Fatalf("first use = %v, %d requests", err, *calls)
	}

	body, err := os.ReadFile(file) //nolint:gosec // a file under the test directory
	if err != nil || !strings.Contains(string(body), "104.16.0.0/12") {
		t.Fatalf("cached file = %q %v", body, err)
	}

	// A fresh table, as after a restart: the file answers.
	if got, err := table(srv.URL, &now).Prefixes(t.Context(), x, "cloudflare"); err != nil || len(got) != 1 || *calls != 1 {
		t.Errorf("from the file = %v %v, %d requests", got, err, *calls)
	}

	// Three days old: fetched again and rewritten.
	old := now.Add(-72 * time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := table(srv.URL, &now).Prefixes(t.Context(), x, "cloudflare"); err != nil || *calls != 2 {
		t.Errorf("an old file = %v, %d requests, want a refresh", err, *calls)
	}

	if info, err := os.Stat(file); err != nil || info.ModTime().Before(now.Add(-time.Hour)) {
		t.Errorf("the refresh did not rewrite the file: %v", err)
	}

	// Old again and the feed is down: the old copy answers with a warning.
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	down["/v4"] = true
	if got, err := table(srv.URL, &now).Prefixes(t.Context(), x, "cloudflare"); err != nil || len(got) != 1 || len(*log) == 0 {
		t.Errorf("down with an old copy = %v %v, log %q", got, err, *log)
	}

	// Down and nothing downloaded before.
	down["/fastly"] = true
	if _, err := table(srv.URL, &now).Prefixes(t.Context(), x, "fastly"); err == nil {
		t.Error("a dead feed with no copy passed")
	}
}
