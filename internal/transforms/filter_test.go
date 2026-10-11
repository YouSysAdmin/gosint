// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"testing"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/yousysadmin/gosint/internal/core/transform"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/filter"
	"github.com/yousysadmin/gosint/internal/transforms/internal/props"
)

// fakeRanges is a range source with one provider.
type fakeRanges struct {
	name     string
	prefixes []netip.Prefix
}

func (f fakeRanges) Names() []string { return []string{f.name} }

func (f fakeRanges) Prefixes(_ context.Context, _ *transform.Exec, provider string) ([]netip.Prefix, error) {
	return f.prefixes, nil
}

// fakeWeb answers a probe by host with a fixed status and refuses the
// rest, so no test reaches the network.
type fakeWeb map[string]int

func (f fakeWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	status, ok := f[r.URL.Hostname()]
	if !ok {
		return nil, errors.New("connection refused")
	}

	return &http.Response{StatusCode: status, Body: http.NoBody, Header: http.Header{}, Request: r}, nil
}

// labels is what a run wrote, by label, which is what the next step of
// a flow receives.
func labels(x *transform.Exec) []string {
	var out []string
	for _, n := range x.Written() {
		out = append(out, n.Label)
	}

	slices.Sort(out)

	return out
}

// A filter passes on what it keeps and only marks what it drops: the
// dropped stay in the sketch, flagged, and do not reach the next step.
func TestIPDropCDNNarrowsAndMarks(t *testing.T) {
	h := newHarness(t)
	ranges := fakeRanges{name: "cloudflare", prefixes: []netip.Prefix{netip.MustParsePrefix("104.16.0.0/12")}}
	cdn := h.node("ip", map[string]any{"address": "104.16.1.1"})
	own := h.node("ip", map[string]any{"address": "8.8.8.8"})

	x := h.run(filter.IPDropCDN{Ranges: ranges}, map[string]string{"providers": "cloudflare", "mode": "drop"}, cdn, own)
	if got := labels(x); !slices.Equal(got, []string{"8.8.8.8"}) {
		t.Errorf("passed on %v, want the address outside the CDN", got)
	}

	marked, err := h.st.Graph.Node(t.Context(), h.sketch, cdn.ID)
	if err != nil || marked == nil || marked.Properties["cdn"] != "cloudflare" || marked.Flag != "yellow" {
		t.Errorf("dropped address = %+v %v, want cdn: cloudflare and a yellow flag", marked, err)
	}

	// What it keeps is handed on, not written again: a filter did not
	// find it, so the count stays at zero and its last seen is untouched.
	if nodes, _ := x.Counts(); nodes != 0 {
		t.Errorf("the filter wrote %d nodes", nodes)
	}

	if kept, err := h.st.Graph.Node(t.Context(), h.sketch, own.ID); err != nil || kept == nil || kept.Metadata["seen"] != nil {
		t.Errorf("kept address = %+v %v, want no seen from the filter", kept, err)
	}

	// Inverted: which of these sit behind the CDN.
	keep := h.run(filter.IPDropCDN{Ranges: ranges}, map[string]string{"providers": "cloudflare", "mode": "keep"}, cdn, own)
	if got := labels(keep); !slices.Equal(got, []string{"104.16.1.1"}) {
		t.Errorf("keep mode passed on %v", got)
	}
}

func TestIPPublicOnly(t *testing.T) {
	h := newHarness(t)
	x := h.run(filter.IPPublicOnly{}, nil,
		h.node("ip", map[string]any{"address": "10.0.0.1"}),
		h.node("ip", map[string]any{"address": "8.8.8.8"}),
		h.node("ip", map[string]any{"address": "127.0.0.1"}))
	if got := labels(x); !slices.Equal(got, []string{"8.8.8.8"}) {
		t.Errorf("passed on %v", got)
	}
}

func TestDomainRootOnly(t *testing.T) {
	h := newHarness(t)
	sub := h.node("domain", map[string]any{"domain": "a.b.example.co.uk"})
	root := h.node("domain", map[string]any{"domain": "example.co.uk"})

	if got := labels(h.run(filter.DomainRootOnly{}, nil, sub, root)); !slices.Equal(got, []string{"example.co.uk"}) {
		t.Errorf("root mode passed on %v", got)
	}

	if got := labels(h.run(filter.DomainRootOnly{}, map[string]string{"mode": "sub"}, sub, root)); !slices.Equal(got, []string{"a.b.example.co.uk"}) {
		t.Errorf("sub mode passed on %v", got)
	}
}

func TestDomainAliveByDNS(t *testing.T) {
	h := newHarness(t)
	h.resolver = fakeDNS(t, map[string]answer{
		"alive.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		},
	})
	h.web = fakeWeb{"alive.test": 200}

	dead := h.node("domain", map[string]any{"domain": "dead.test"})
	x := h.run(filter.DomainAlive{}, nil, h.node("domain", map[string]any{"domain": "alive.test"}), dead)
	if got := labels(x); !slices.Equal(got, []string{"alive.test"}) {
		t.Errorf("passed on %v", got)
	}

	marked, _ := h.st.Graph.Node(t.Context(), h.sketch, dead.ID)
	if marked == nil || marked.Properties["alive"] != false || marked.Flag != "red" ||
		marked.Properties["http_status"] != nil || !strings.HasPrefix(props.Str(marked.Properties, "check_error"), "DNS: ") {
		t.Errorf("dead domain = %+v", marked)
	}

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "1 of 2 domains are alive") }) {
		t.Errorf("log = %q", h.log)
	}
}

// with_subdomains walks the domains under the selected one, each result
// is logged with its reason, a live domain is written alive with the
// time of the check, and a red flag a dead run left on it goes.
func TestDomainAliveWalksSubdomainsAndExplains(t *testing.T) {
	h := newHarness(t)
	h.resolver = fakeDNS(t, map[string]answer{
		"root.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		},
		"www.root.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 11}})
		},
	})

	h.web = fakeWeb{"root.test": 200, "www.root.test": 200}

	root := h.node("domain", map[string]any{"domain": "root.test"})
	www := h.node("domain", map[string]any{"domain": "www.root.test"})
	gone := h.node("domain", map[string]any{"domain": "gone.www.root.test"})
	owner := h.node("organization", map[string]any{"name": "Root Org"})
	// An MX target is a domain drawn from the root, not a subdomain.
	mx := h.node("domain", map[string]any{"domain": "aspmx.example"})
	for _, e := range []graphmodel.Edge{
		{SourceID: root.ID, TargetID: www.ID, Label: "HAS_SUBDOMAIN"},
		{SourceID: www.ID, TargetID: gone.ID, Label: "HAS_SUBDOMAIN"},
		{SourceID: owner.ID, TargetID: root.ID, Label: "OWNS"},
		{SourceID: root.ID, TargetID: mx.ID, Label: "HAS_MX"},
	} {
		if _, err := h.st.Graph.UpsertEdge(t.Context(), h.sketch, e); err != nil {
			t.Fatal(err)
		}
	}

	// An earlier run found www dead.
	red := "red"
	if _, err := h.st.Graph.UpdateNode(t.Context(), h.sketch, www.ID, graphmodel.NodePatch{
		Properties: map[string]any{"alive": false}, Flag: &red}); err != nil {
		t.Fatal(err)
	}

	x := h.run(filter.DomainAlive{}, map[string]string{"with_subdomains": "true"}, root)
	if got := labels(x); !slices.Equal(got, []string{"root.test", "www.root.test"}) {
		t.Errorf("passed on %v", got)
	}

	now, _ := h.st.Graph.Node(t.Context(), h.sketch, www.ID)
	if now.Properties["alive"] != true || now.Properties["checked_at"] == nil || now.Flag != "" {
		t.Errorf("www after the check = %+v flag %q", now.Properties, now.Flag)
	}

	for _, want := range []string{
		"2 subdomains under the selected domains are checked too",
		"root.test is alive: resolves to 192.0.2.10",
		"gone.www.root.test is dead: DNS: ",
		"2 of 3 domains are alive",
	} {
		if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("no %q in %q", want, h.log)
		}
	}
}

// The flag reads the verdict: none for a 200, yellow for any other
// answer or no web server behind a name that resolves, with the status
// and the error written onto the domain, and a later 200 clears a
// yellow flag like a red one.
func TestDomainAliveFlagsByStatus(t *testing.T) {
	h := newHarness(t)
	answers := map[string]answer{}
	for _, name := range []string{"ok.test", "gone.test", "forbidden.test", "silent.test"} {
		answers[name+". TypeA"] = func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		}
	}

	h.resolver = fakeDNS(t, answers)
	h.web = fakeWeb{"ok.test": 200, "gone.test": 404, "forbidden.test": 403}

	ok := h.node("domain", map[string]any{"domain": "ok.test"})
	gone := h.node("domain", map[string]any{"domain": "gone.test"})
	forbidden := h.node("domain", map[string]any{"domain": "forbidden.test"})
	silent := h.node("domain", map[string]any{"domain": "silent.test"})

	// An earlier run left ok.test yellow and silent.test red.
	for n, flag := range map[*graphmodel.Node]string{&ok: "yellow", &silent: "red"} {
		got, err := h.st.Graph.UpdateNode(t.Context(), h.sketch, n.ID, graphmodel.NodePatch{Flag: &flag})
		if err != nil {
			t.Fatal(err)
		}

		*n = *got
	}

	x := h.run(filter.DomainAlive{}, nil, ok, gone, forbidden, silent)
	if got := labels(x); !slices.Equal(got, []string{"forbidden.test", "gone.test", "ok.test", "silent.test"}) {
		t.Errorf("dns mode passed on %v", got)
	}

	for _, want := range []struct {
		node   graphmodel.Node
		flag   string
		status any
		errHas string
	}{
		{ok, "", 200, ""},
		{gone, "yellow", 404, ""},
		{forbidden, "yellow", 403, ""},
		{silent, "yellow", nil, "connection refused"},
	} {
		got, _ := h.st.Graph.Node(t.Context(), h.sketch, want.node.ID)
		if got.Flag != want.flag || got.Properties["alive"] != true {
			t.Errorf("%s: flag %q alive %v", want.node.Label, got.Flag, got.Properties["alive"])
		}

		if s := got.Properties["http_status"]; want.status == nil && s != nil || want.status != nil && s != float64(want.status.(int)) {
			t.Errorf("%s: http_status = %v, want %v", want.node.Label, s, want.status)
		}

		if e := props.Str(got.Properties, "check_error"); !strings.Contains(e, want.errHas) || (want.errHas == "" && e != "") {
			t.Errorf("%s: check_error = %q", want.node.Label, e)
		}
	}

	// In http mode the silent one is dropped, still yellow.
	x = h.run(filter.DomainAlive{}, map[string]string{"check": "http"}, ok, silent)
	if got := labels(x); !slices.Equal(got, []string{"ok.test"}) {
		t.Errorf("http mode passed on %v", got)
	}
}

// Every filter gives what it takes, so it fits anywhere in a flow.
func TestFiltersTakeWhatTheyGive(t *testing.T) {
	for _, e := range filter.All() {
		if m := e.Meta(); m.Input != m.Output || !strings.HasPrefix(m.Description, "Narrows:") {
			t.Errorf("%s: input %q output %q, description %q", m.Name, m.Input, m.Output, m.Description)
		}
	}
}

var _ graphmodel.Node
