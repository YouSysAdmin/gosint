// Gosint, Copyright (c) 2026 YouSysAdmin

// Package cdnranges knows which addresses belong to the CDNs and clouds
// that publish their ranges: Cloudflare, Fastly, AWS and Google Cloud.
// An address behind one of them says nothing about the host behind it,
// so a flow drops them before probing. The feeds are downloaded on first
// use into the worker's cache directory, read from there while the file
// is younger than the TTL, and fetched again after. A onDisk file answers
// when a feed cannot be reached.
package cdnranges

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
)

// Provider is one publisher of ranges.
type Provider struct {
	Name string

	// URLs are fetched in turn and their prefixes joined.
	URLs []string

	// Parse reads one feed into prefixes.
	Parse func(body []byte) ([]netip.Prefix, error)
}

// The bounds of one feed read: AWS is about 2 MB.
const (
	maxFeed     = 8 << 20
	feedTimeout = 30 * time.Second

	// DefaultTTL is how long downloaded ranges are used before they are
	// fetched again, judged by the file's modification time.
	DefaultTTL = 48 * time.Hour

	// retryAfter is how soon a failed refresh is tried again while a
	// onDisk copy answers.
	retryAfter = time.Hour

	// clockSkew is how far ahead of this clock a file's time may be and
	// still count as written now (another node's clock, a shared
	// directory). Further ahead it is treated as stale.
	clockSkew = time.Minute
)

// Table caches the prefixes of every provider.
type Table struct {
	Providers []Provider
	TTL       time.Duration

	// Now is the clock, swapped by tests.
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]entry

	// flight is the fetch in progress per provider, so concurrent runs
	// wait for one download instead of each starting their own.
	flight map[string]chan struct{}
}

type entry struct {
	prefixes []netip.Prefix
	until    time.Time
}

// Public is the table of the published feeds.
var Public = &Table{Providers: []Provider{Cloudflare, Fastly, AWS, GCP}, TTL: DefaultTTL}

// Names lists the providers a table knows, in order.
func (t *Table) Names() []string {
	out := make([]string, 0, len(t.Providers))
	for _, p := range t.Providers {
		out = append(out, p.Name)
	}

	return out
}

// Prefixes is a provider's ranges: from memory, else from the file in
// x.CacheDir while it is fresh, else fetched through the fixed-API client
// and written there, else the onDisk file with a warning.
func (t *Table) Prefixes(ctx context.Context, x *transform.Exec, name string) ([]netip.Prefix, error) {
	var provider *Provider
	for i := range t.Providers {
		if strings.EqualFold(t.Providers[i].Name, name) {
			provider = &t.Providers[i]
		}
	}

	if provider == nil {
		return nil, fmt.Errorf("unknown provider %q, known: %s", name, strings.Join(t.Names(), ", "))
	}

	now := time.Now
	if t.Now != nil {
		now = t.Now
	}

	for {
		t.mu.Lock()
		if e, ok := t.cache[provider.Name]; ok && now().Before(e.until) {
			t.mu.Unlock()

			return e.prefixes, nil
		}

		wait, busy := t.flight[provider.Name]
		if !busy {
			if t.flight == nil {
				t.flight = map[string]chan struct{}{}
			}

			wait = make(chan struct{})
			t.flight[provider.Name] = wait
			t.mu.Unlock()

			break
		}

		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}

	defer func() {
		t.mu.Lock()
		close(t.flight[provider.Name])
		delete(t.flight, provider.Name)
		t.mu.Unlock()
	}()

	ttl := t.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}

	file := ""
	if x.CacheDir != "" {
		file = filepath.Join(x.CacheDir, "cdnranges", strings.ToLower(provider.Name)+".txt")
	}

	onDisk, saved, readErr := readFile(file)
	if readErr == nil && !saved.After(now().Add(clockSkew)) && now().Sub(saved) < ttl {
		t.keep(provider.Name, onDisk, saved.Add(ttl))

		return onDisk, nil
	}

	prefixes, err := fetch(ctx, x.HTTP, *provider)
	if err != nil {
		if readErr != nil {
			return nil, fmt.Errorf("%s ranges: %w, and no copy was downloaded before", provider.Name, err)
		}

		x.Warn("%s ranges: %v, using the copy from %s", provider.Name, err, saved.UTC().Format(time.DateOnly))
		t.keep(provider.Name, onDisk, now().Add(min(ttl, retryAfter)))

		return onDisk, nil
	}

	if file != "" {
		if err := writeFile(file, provider.Name, prefixes); err != nil {
			x.Warn("%s ranges: not cached: %v", provider.Name, err)
		}
	}

	t.keep(provider.Name, prefixes, now().Add(ttl))

	return prefixes, nil
}

func (t *Table) keep(name string, prefixes []netip.Prefix, until time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cache == nil {
		t.cache = map[string]entry{}
	}

	t.cache[name] = entry{prefixes: prefixes, until: until}
}

// readFile reads a downloaded copy and when it was written.
func readFile(file string) ([]netip.Prefix, time.Time, error) {
	if file == "" {
		return nil, time.Time{}, os.ErrNotExist
	}

	info, err := os.Stat(file)
	if err != nil {
		return nil, time.Time{}, err
	}

	body, err := os.ReadFile(file) //nolint:gosec // a path under the configured cache directory
	if err != nil {
		return nil, time.Time{}, err
	}

	prefixes, err := ParseLines(body)
	if err != nil || len(prefixes) == 0 {
		return nil, time.Time{}, fmt.Errorf("%s holds no ranges", file)
	}

	return prefixes, info.ModTime(), nil
}

// writeFile replaces the copy at file, through a temporary file so a
// reader never sees half of it.
func writeFile(file, name string, prefixes []netip.Prefix) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o750); err != nil {
		return err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s ranges, downloaded %s\n", name, time.Now().UTC().Format(time.RFC3339))
	for _, p := range prefixes {
		b.WriteString(p.String() + "\n")
	}

	tmp, err := os.CreateTemp(filepath.Dir(file), "."+name+"-*")
	if err != nil {
		return err
	}

	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename

	if _, err := tmp.Write(b.Bytes()); err != nil {
		_ = tmp.Close()

		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), file)
}

// fetch reads every URL of the provider.
func fetch(ctx context.Context, client *http.Client, p Provider) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, u := range p.URLs {
		prefixes, err := fetchOne(ctx, client, u, p.Parse)
		if err != nil {
			return nil, err
		}

		out = append(out, prefixes...)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("%s published no ranges", p.Name)
	}

	return out, nil
}

func fetchOne(ctx context.Context, client *http.Client, u string, parse func([]byte) ([]netip.Prefix, error)) ([]netip.Prefix, error) {
	ctx, cancel := context.WithTimeout(ctx, feedTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, transform.RedactURLError(err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFeed))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", req.URL.Host, resp.Status)
	}

	return parse(body)
}

// Contains reports whether a sits in one of prefixes.
func Contains(prefixes []netip.Prefix, a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range prefixes {
		if p.Contains(a) {
			return true
		}
	}

	return false
}

// ParseLines reads one prefix per line, as Cloudflare publishes and the
// downloaded copies are kept. Blank lines and comments are skipped.
func ParseLines(body []byte) ([]netip.Prefix, error) {
	var out []netip.Prefix
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p, err := netip.ParsePrefix(line)
		if err != nil {
			return nil, fmt.Errorf("bad range %q", line)
		}

		out = append(out, p)
	}

	return out, sc.Err()
}

// parseStrings turns prefix strings into prefixes, skipping empties.
func parseStrings(list []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(list))
	for _, s := range list {
		if s == "" {
			continue
		}

		p, err := netip.ParsePrefix(s)
		if err != nil {
			return nil, fmt.Errorf("bad range %q", s)
		}

		out = append(out, p)
	}

	return out, nil
}

// The published feeds.
var (
	Cloudflare = Provider{Name: "cloudflare",
		URLs:  []string{"https://www.cloudflare.com/ips-v4", "https://www.cloudflare.com/ips-v6"},
		Parse: ParseLines}

	Fastly = Provider{Name: "fastly", URLs: []string{"https://api.fastly.com/public-ip-list"},
		Parse: func(body []byte) ([]netip.Prefix, error) {
			var feed struct {
				Addresses     []string `json:"addresses"`
				IPv6Addresses []string `json:"ipv6_addresses"`
			}

			if err := json.Unmarshal(body, &feed); err != nil {
				return nil, err
			}

			return parseStrings(append(feed.Addresses, feed.IPv6Addresses...))
		}}

	AWS = Provider{Name: "aws", URLs: []string{"https://ip-ranges.amazonaws.com/ip-ranges.json"},
		Parse: func(body []byte) ([]netip.Prefix, error) {
			var feed struct {
				Prefixes []struct {
					IPPrefix string `json:"ip_prefix"`
				} `json:"prefixes"`
				IPv6Prefixes []struct {
					IPv6Prefix string `json:"ipv6_prefix"`
				} `json:"ipv6_prefixes"`
			}

			if err := json.Unmarshal(body, &feed); err != nil {
				return nil, err
			}

			list := make([]string, 0, len(feed.Prefixes)+len(feed.IPv6Prefixes))
			for _, p := range feed.Prefixes {
				list = append(list, p.IPPrefix)
			}

			for _, p := range feed.IPv6Prefixes {
				list = append(list, p.IPv6Prefix)
			}

			return parseStrings(list)
		}}

	GCP = Provider{Name: "gcp", URLs: []string{"https://www.gstatic.com/ipranges/cloud.json"},
		Parse: func(body []byte) ([]netip.Prefix, error) {
			var feed struct {
				Prefixes []struct {
					IPv4Prefix string `json:"ipv4Prefix"`
					IPv6Prefix string `json:"ipv6Prefix"`
				} `json:"prefixes"`
			}

			if err := json.Unmarshal(body, &feed); err != nil {
				return nil, err
			}

			list := make([]string, 0, len(feed.Prefixes))
			for _, p := range feed.Prefixes {
				list = append(list, p.IPv4Prefix+p.IPv6Prefix)
			}

			return parseStrings(list)
		}}
)
