// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"cmp"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/store"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
	"github.com/yousysadmin/gosint/internal/transforms"
	"github.com/yousysadmin/gosint/internal/transforms/domain"
	"github.com/yousysadmin/gosint/internal/transforms/email"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nmap"
	"github.com/yousysadmin/gosint/internal/transforms/ip"
	"github.com/yousysadmin/gosint/internal/transforms/location"
	"github.com/yousysadmin/gosint/internal/transforms/website"
)

type lines []string

func (l *lines) Log(level logmodel.Level, message string, _ map[string]any) {
	*l = append(*l, string(level)+": "+message)
}

// harness is one sketch to transform on, on a fresh sqlite database.
type harness struct {
	resolver *net.Resolver

	// web stands in for the sites a run probes, the real client when nil.
	web http.RoundTripper

	// allowPrivate is the private address switch of every tool.
	allowPrivate bool

	// scan names the run, scan-1 unless a test changes it.
	scan   string
	t      *testing.T
	st     *store.Store
	sketch string
	log    lines
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	st := backend.BindStore(dbtest.SQLite(t), nil)
	ctx, now := t.Context(), time.Now().UTC()
	u := &usermodel.User{ID: ids.New(), Email: "a@x.io", PasswordHash: "x", Active: true, CreatedAt: now, UpdatedAt: now}
	inv := &invmodel.Investigation{ID: ids.New(), Name: "c", Status: "active", OwnerID: u.ID, CreatedAt: now, UpdatedAt: now}
	sk := &sketchmodel.Sketch{ID: ids.New(), InvestigationID: inv.ID, Title: "s", Status: "active", CreatedAt: now, UpdatedAt: now}

	for _, err := range []error{st.User.Create(ctx, u), st.Investigation.Create(ctx, inv), st.Sketch.Create(ctx, sk)} {
		if err != nil {
			t.Fatal(err)
		}
	}

	return &harness{t: t, st: st, sketch: sk.ID, scan: "scan-1"}
}

func (h *harness) node(typ string, props map[string]any) graphmodel.Node {
	h.t.Helper()

	label, err := types.Builtin().Build(typ, props)
	if err != nil {
		h.t.Fatal(err)
	}

	out, err := h.st.Graph.UpsertNodes(h.t.Context(), h.sketch, []graphmodel.Node{{Type: typ, Key: types.Builtin().Key(typ, props), Label: label, Properties: props}})
	if err != nil {
		h.t.Fatal(err)
	}

	return out[0]
}

// run runs the transform on inputs and hands back its Exec, so a test
// can read what it wrote (a filter's Written is what flows on).
func (h *harness) run(e transform.Transform, params map[string]string, inputs ...graphmodel.Node) *transform.Exec {
	h.t.Helper()

	web := transform.DefaultHTTPClient("gosint-test")
	if h.web != nil {
		web = &http.Client{Transport: h.web}
	}

	x := transform.NewExec(transform.ExecConfig{
		SketchID: h.sketch, Graph: h.st.Graph, Types: types.Builtin(), Log: &h.log, Params: params,
		HTTP: transform.DefaultHTTPClient("gosint-test"), Web: web,
		Resolver: cmp.Or(h.resolver, net.DefaultResolver), Tools: h.tools(),
		ScanID: h.scan, Transform: e.Meta().Name, Files: h.st.Attachment,
	})
	if err := e.Run(h.t.Context(), x, inputs); err != nil {
		h.t.Fatalf("%s: %v", e.Meta().Name, err)
	}

	return x
}

// edges lists "source label -LABEL-> target label" for the sketch.
func (h *harness) edges() []string {
	h.t.Helper()

	nodes, edges, err := h.st.Graph.Graph(h.t.Context(), h.sketch)
	if err != nil {
		h.t.Fatal(err)
	}

	label := map[string]string{}
	for _, n := range nodes {
		label[n.ID] = n.Label
	}

	var out []string
	for _, e := range edges {
		out = append(out, label[e.SourceID]+" -"+e.Label+"-> "+label[e.TargetID])
	}

	slices.Sort(out)

	return out
}

func (h *harness) expectEdges(want ...string) {
	h.t.Helper()

	slices.Sort(want)
	if got := h.edges(); !slices.Equal(got, want) {
		h.t.Errorf("edges:\n got  %q\n want %q\nlog: %q", got, want, h.log)
	}
}

func TestEveryTransformIsWellDeclared(t *testing.T) {
	reg := transform.NewRegistry(transforms.All()...)
	for _, e := range reg.For("") {
		m := e.Meta()
		// Output "" is a transform drawing whatever its input's fields name.
		if m.Name == "" || m.Category == "" || m.Description == "" || (m.Output == "" && m.Input != "") {
			t.Errorf("%T: incomplete meta %+v", e, m)
		}

		for _, typ := range []string{m.Input, m.Output} {
			if _, ok := types.Builtin().Get(typ); typ != "" && !ok {
				t.Errorf("%s names unknown type %q", m.Name, typ)
			}
		}
	}
}

func TestLocalTransforms(t *testing.T) {
	h := newHarness(t)
	mail := h.node("email", map[string]any{"email": "john.doe@example.co.uk"})
	site := h.node("website", map[string]any{"url": "https://www.shop.example.co.uk:8443/cart"})
	sub := h.node("domain", map[string]any{"domain": "a.b.example.co.uk"})

	h.run(email.EmailToDomain{}, nil, mail)
	h.run(email.EmailToUsername{}, nil, mail)
	h.run(website.WebsiteToDomain{}, nil, site)
	h.run(domain.DomainToRootDomain{}, nil, sub)

	h.expectEdges(
		"john.doe@example.co.uk -HAS_DOMAIN-> example.co.uk",
		"john.doe@example.co.uk -HAS_USERNAME-> john.doe",
		"https://www.shop.example.co.uk:8443/cart -HAS_DOMAIN-> shop.example.co.uk",
		"example.co.uk -HAS_SUBDOMAIN-> a.b.example.co.uk",
	)
}

func TestDomainToIPResolves(t *testing.T) {
	h := newHarness(t)
	h.resolver = fakeDNS(t, map[string]answer{
		"acme.test. TypeA": func(b *dnsmessage.Builder, q dnsmessage.Question) {
			_ = b.AResource(header(q), dnsmessage.AResource{A: [4]byte{192, 0, 2, 10}})
		},
	})

	h.run(domain.DomainToIP{}, nil, h.node("domain", map[string]any{"domain": "acme.test"}), h.node("domain", map[string]any{"domain": "nonexistent.invalid"}))
	h.expectEdges("acme.test -RESOLVES_TO-> 192.0.2.10")

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.HasPrefix(l, "warning: nonexistent.invalid") }) {
		t.Errorf("an unresolvable domain was not reported: %q", h.log)
	}
}

// api is a fake of the third-party APIs, by path.
func api(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) string {
	t.Helper()

	// The longest matching prefix wins, so a route under another one
	// is reached whatever the map order.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var best string
		for prefix := range routes {
			if strings.HasPrefix(r.URL.Path, prefix) && len(prefix) > len(best) {
				best = prefix
			}
		}

		if h, ok := routes[best]; ok {
			h(w, r)

			return
		}

		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/"
}

func TestSubdomainsFromCertificates(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") != "%.example.com" {
				t.Errorf("crt.sh query = %q", r.URL.Query().Get("q"))
			}

			_, _ = w.Write([]byte(`[
				{"name_value": "www.example.com\nmail.example.com"},
				{"name_value": "*.example.com"},
				{"name_value": "evilexample.com"},
				{"name_value": "WWW.example.com"}
			]`))
		},
	})

	h.run(domain.DomainToSubdomains{BaseURL: base}, nil, h.node("domain", map[string]any{"domain": "example.com"}))
	h.expectEdges("example.com -HAS_SUBDOMAIN-> mail.example.com", "example.com -HAS_SUBDOMAIN-> www.example.com")
}

func TestIPToInfosGeolocatesInPlace(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/8.8.8.8": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"success","lat":37.4,"lon":-122.1,"country":"United States","city":"Mountain View","isp":"Google LLC"}`))
		},
		"/10.0.0.1": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"status":"fail","message":"private range"}`))
		},
	})

	addr := h.node("ip", map[string]any{"address": "8.8.8.8"})
	h.run(ip.IPToInfos{BaseURL: base}, nil, addr, h.node("ip", map[string]any{"address": "10.0.0.1"}))

	n, err := h.st.Graph.Node(t.Context(), h.sketch, addr.ID)
	if err != nil {
		t.Fatal(err)
	}

	if n.Properties["city"] != "Mountain View" || n.Properties["isp"] != "Google LLC" {
		t.Errorf("ip after enrichment = %v", n.Properties)
	}

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "private range") }) {
		t.Errorf("the refused address was not reported: %q", h.log)
	}
}

func TestEmailToGravatar(t *testing.T) {
	h := newHarness(t)
	// md5("someone@example.com")
	const hash = "16d113840f999444259f73bac9ab8b10"
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/avatar/" + hash: func(w http.ResponseWriter, _ *http.Request) {},
		"/" + hash + ".json": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"entry":[{"displayName":"Some One","currentLocation":"Paris"}]}`))
		},
	})

	h.run(email.EmailToGravatar{BaseURL: base}, nil,
		h.node("email", map[string]any{"email": "Someone@Example.com"}),
		h.node("email", map[string]any{"email": "nobody@example.com"}))

	h.expectEdges("Someone@example.com -HAS_GRAVATAR-> Some One")
}

func TestEmailToBreaches(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/breachedaccount/leaked@example.com": func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("hibp-api-key") != "secret" {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			_, _ = w.Write([]byte(`[{"Name":"Adobe","Title":"Adobe","Domain":"adobe.com","PwnCount":152445165,
				"DataClasses":["Email addresses","Passwords"],"IsVerified":true}]`))
		},
	})

	h.run(email.EmailToBreaches{}, map[string]string{"HIBP_API_KEY": "secret", "HIBP_API_URL": base + "breachedaccount"},
		h.node("email", map[string]any{"email": "leaked@example.com"}),
		h.node("email", map[string]any{"email": "clean@example.com"}))

	h.expectEdges("leaked@example.com -FOUND_IN_BREACH-> Adobe")
}

func TestLocationToCoordinates(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("street") == "1 Main St" && q.Get("city") == "Paris" {
				_, _ = w.Write([]byte(`[{"lat":"48.8566","lon":"2.3522","display_name":"Paris"}]`))

				return
			}

			_, _ = w.Write([]byte(`[]`))
		},
	})

	paris := h.node("location", map[string]any{"address": "1 Main St", "city": "Paris", "country": "FR", "zip": "75001"})
	lost := h.node("location", map[string]any{"address": "nowhere", "city": "Atlantis", "country": "AT", "zip": "0"})
	h.run(location.LocationToCoordinates{BaseURL: base}, nil, paris, lost)

	n, err := h.st.Graph.Node(t.Context(), h.sketch, paris.ID)
	if err != nil {
		t.Fatal(err)
	}

	if n.Properties["latitude"] != 48.8566 || n.Properties["longitude"] != 2.3522 || n.Properties["city"] != "Paris" {
		t.Errorf("location after geocoding = %v", n.Properties)
	}

	if !slices.ContainsFunc(h.log, func(l string) bool { return strings.Contains(l, "no match") }) {
		t.Errorf("the unknown address was not reported: %q", h.log)
	}
}

// tools is the tool settings of a run, every switch as allowPrivate.
func (h *harness) tools() tool.Settings {
	return tool.Settings{tool.WebName: {AllowPrivate: h.allowPrivate}, nmap.ToolName: {AllowPrivate: h.allowPrivate}}
}
