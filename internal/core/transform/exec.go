// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/domain/store"
	"github.com/yousysadmin/gosint/internal/models/attachment"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
)

// Logger is where a transform's log lines go, the scan's run.
type Logger interface {
	Log(level logmodel.Level, message string, data map[string]any)
}

// Exec is what a running transform can reach: its parameters, the
// network, the log, and the sketch it writes into.
type Exec struct {
	// HTTP has timeouts and identifies gosint. Use it for fixed APIs.
	HTTP *http.Client

	// Web fetches sites that entities name. It refuses internal
	// addresses, see TargetHTTPClient.
	Web *http.Client

	// Resolver is the DNS resolver.
	Resolver *net.Resolver

	// CacheDir keeps downloaded reference data across runs, empty for
	// none.
	CacheDir string

	// AllowPrivate lets a transform contact private addresses on an
	// entity's behalf, a lab setting: the web tool's switch.
	AllowPrivate bool

	// Tools are the administrator's tool settings at the run's start,
	// a program's path and its own private address switch.
	Tools tool.Settings

	// ScanID, Transform and Step name the run, written as the origin of
	// every node and edge it adds.
	ScanID    string
	Transform string
	Step      string

	params   map[string]string
	sketchID string
	graph    store.GraphStore
	files    store.AttachmentStore
	types    *types.Registry
	log      Logger

	nodes, edges int

	// written are the nodes the run wrote, by id, for a flow to hand to
	// its next step.
	written map[string]graphmodel.Node
}

// ExecConfig is what NewExec needs.
type ExecConfig struct {
	SketchID string
	Graph    store.GraphStore
	Types    *types.Registry
	Log      Logger
	Params   map[string]string
	HTTP     *http.Client
	Web      *http.Client
	Resolver *net.Resolver
	CacheDir string
	Tools    tool.Settings

	// ScanID, Transform and Step name the run for the origin of what it
	// writes.
	ScanID    string
	Transform string
	Step      string

	// Files is the store of the files attached to entities, for a
	// transform reading what they say about themselves. May be nil.
	Files store.AttachmentStore
}

// guarded is the Web client an Exec gets when none is configured, so a
// wiring that forgets it still refuses internal addresses, or, with
// the web tool allowed private addresses, reaches them like the rest
// of the run. A test reaching
// an httptest server sets Web explicitly.
var (
	guarded = sync.OnceValue(func() *http.Client { return TargetHTTPClient("gosint", Never) })
	open    = sync.OnceValue(func() *http.Client { return TargetHTTPClient("gosint", Always) })
)

func defaultWeb(allowPrivate bool) *http.Client {
	if allowPrivate {
		return open()
	}

	return guarded()
}

// NewExec builds the context one transform run gets. An Exec serves one
// run on one goroutine, it is not safe for concurrent use.
func NewExec(cfg ExecConfig) *Exec {
	allowPrivate := cfg.Tools.Get(tool.WebName).AllowPrivate

	return &Exec{
		HTTP:     cfg.HTTP,
		Web:      cmp.Or(cfg.Web, defaultWeb(allowPrivate)),
		Resolver: cfg.Resolver,
		CacheDir: cfg.CacheDir,

		AllowPrivate: allowPrivate,
		Tools:        cfg.Tools,
		ScanID:       cfg.ScanID,
		Transform:    cfg.Transform,
		Step:         cfg.Step,
		params:       cfg.Params,
		sketchID:     cfg.SketchID,
		graph:        cfg.Graph,
		files:        cfg.Files,
		types:        cfg.Types,
		log:          cfg.Log,
	}
}

// DefaultHTTPClient is the client transforms get unless a test swaps it.
// Its timeout is a ceiling, each request bounds itself below it (see
// httpx.GetJSONWithin), since a slow source such as crt.sh needs longer
// than an ordinary API should get.
func DefaultHTTPClient(userAgent string) *http.Client {
	return &http.Client{
		Timeout:   3 * time.Minute,
		Transport: userAgentTransport{agent: userAgent, next: http.DefaultTransport},
	}
}

type userAgentTransport struct {
	agent string
	next  http.RoundTripper
}

// RoundTrip sets the User-Agent unless the request set its own.
func (t userAgentTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Header.Get("User-Agent") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("User-Agent", t.agent)
	}

	return t.next.RoundTrip(r)
}

// Param is a resolved parameter, "" when unset.
func (x *Exec) Param(name string) string { return x.params[name] }

// Info logs a line to the sketch's log.
func (x *Exec) Info(format string, args ...any) {
	x.log.Log(logmodel.LevelInfo, fmt.Sprintf(format, args...), nil)
}

// Warn logs a problem with one input that does not stop the run.
func (x *Exec) Warn(format string, args ...any) {
	x.log.Log(logmodel.LevelWarning, fmt.Sprintf(format, args...), nil)
}

// Found logs a graph finding, the line the console's log shows for each
// thing a transform added.
func (x *Exec) Found(format string, args ...any) {
	x.log.Log(logmodel.LevelGraphAppend, fmt.Sprintf(format, args...), nil)
}

// Node writes an entity into the sketch, merging into the node of the
// same type and label when there is one. A value its type refuses is an
// error, which callers log and skip.
func (x *Exec) Node(ctx context.Context, typeName string, props map[string]any) (graphmodel.Node, error) {
	return x.node(ctx, typeName, props, nil)
}

// NodeFrom is Node with the input the entity was found from, recorded
// in its origin.
func (x *Exec) NodeFrom(ctx context.Context, from graphmodel.Node, typeName string, props map[string]any) (graphmodel.Node, error) {
	return x.node(ctx, typeName, props, &from)
}

// origin is this run as the origin of a write, nil outside a scan.
func (x *Exec) origin(from *graphmodel.Node) *graphmodel.Origin {
	if x.ScanID == "" && x.Transform == "" {
		return nil
	}

	o := &graphmodel.Origin{Scan: x.ScanID, Transform: x.Transform, Step: x.Step, Via: "transform", At: time.Now().UTC()}
	if x.Step != "" {
		o.Via = "flow"
	}

	if from != nil {
		o.From, o.FromID = from.Label, from.ID
	}

	return o
}

func (x *Exec) node(ctx context.Context, typeName string, props map[string]any, from *graphmodel.Node) (graphmodel.Node, error) {
	p := maps.Clone(props)
	if p == nil {
		p = map[string]any{}
	}

	label, err := x.types.Build(typeName, p)
	if err != nil {
		return graphmodel.Node{}, fmt.Errorf("%s: %w", typeName, err)
	}

	n := graphmodel.Node{Type: typeName, Key: x.types.Key(typeName, p), Label: label, Properties: p}
	if o := x.origin(from); o != nil {
		// The store keeps the first origin, seen is overwritten each time.
		n.Metadata = map[string]any{graphmodel.OriginKey: o.Map(), graphmodel.SeenKey: o.Map()}
	}

	out, err := x.graph.UpsertNodes(ctx, x.sketchID, []graphmodel.Node{n})
	if err != nil {
		return graphmodel.Node{}, err
	}

	x.nodes++
	if x.written == nil {
		x.written = map[string]graphmodel.Node{}
	}

	x.written[out[0].ID] = out[0]

	return out[0], nil
}

// Pass hands an input on to the next flow step as it is, with no write:
// a filter keeping an entity has not found it again, so its provenance
// and the sketch stay untouched.
func (x *Exec) Pass(n graphmodel.Node) {
	if x.written == nil {
		x.written = map[string]graphmodel.Node{}
	}

	x.written[n.ID] = n
}

// Link writes a directed relationship between two nodes of the sketch.
func (x *Exec) Link(ctx context.Context, from, to graphmodel.Node, label string) error {
	return x.LinkWith(ctx, from, to, label, nil)
}

// LinkWith is Link with properties on the relationship, such as the
// priority of an MX record.
func (x *Exec) LinkWith(ctx context.Context, from, to graphmodel.Node, label string, props map[string]any) error {
	if o := x.origin(nil); o != nil {
		props = maps.Clone(props)
		if props == nil {
			props = map[string]any{}
		}

		props[graphmodel.OriginKey] = o.Map()
	}

	e, err := x.graph.UpsertEdge(ctx, x.sketchID, graphmodel.Edge{SourceID: from.ID, TargetID: to.ID, Label: label, Properties: props})
	if err != nil {
		return err
	}

	if e == nil {
		return fmt.Errorf("link %s: an end is not on the sketch", label)
	}

	x.edges++

	return nil
}

// NodeAndLink writes the entity and links from to it, the common shape
// of a finding. It logs what it added.
func (x *Exec) NodeAndLink(ctx context.Context, from graphmodel.Node, label, typeName string, props map[string]any) (graphmodel.Node, error) {
	n, err := x.NodeFrom(ctx, from, typeName, props)
	if err != nil {
		return n, err
	}

	if err := x.Link(ctx, from, n, label); err != nil {
		return n, err
	}

	x.Found("%s -> %s -> %s", from.Label, label, n.Label)

	return n, nil
}

// Annotate writes properties and a flag colour onto a node WITHOUT
// counting it as written: for a filter marking what it dropped, which
// must not flow to the next step of a flow. props go through the type
// like Node's. flag is one of the console's colours, "" leaves it.
func (x *Exec) Annotate(ctx context.Context, n graphmodel.Node, props map[string]any, flag string) error {
	p := maps.Clone(n.Properties)
	if p == nil {
		p = map[string]any{}
	}

	maps.Copy(p, props)
	label, err := x.types.Build(n.Type, p)
	if err != nil {
		return fmt.Errorf("%s: %w", n.Type, err)
	}

	_, err = x.graph.UpsertNodes(ctx, x.sketchID, []graphmodel.Node{{Type: n.Type, Key: x.types.Key(n.Type, p),
		Label: label, Properties: props, Style: graphmodel.Style{Flag: flag}}})

	return err
}

// Files is the files attached to n, with their metadata. None when the
// run was given no file store.
func (x *Exec) Files(ctx context.Context, n graphmodel.Node) ([]attachment.Attachment, error) {
	if x.files == nil {
		return nil, nil
	}

	return x.files.ListForNode(ctx, x.sketchID, n.ID)
}

// Neighbours is n's surroundings: the nodes one relationship away and
// those relationships, for a transform that follows the graph.
func (x *Exec) Neighbours(ctx context.Context, n graphmodel.Node) ([]graphmodel.Node, []graphmodel.Edge, error) {
	return x.graph.Neighbors(ctx, x.sketchID, n.ID)
}

// Unflag clears n's flag when it is flag, the colour a filter set on an
// earlier run, and leaves a flag somebody else chose.
func (x *Exec) Unflag(ctx context.Context, n graphmodel.Node, flag string) error {
	if n.Flag != flag {
		return nil
	}

	_, err := x.graph.UpdateNode(ctx, x.sketchID, n.ID, graphmodel.NodePatch{Flag: new("")})

	return err
}

// Written lists the nodes the run wrote, each once, in id order. A
// flow feeds them to the next step, so what a filter does not hand on
// does not travel on: a filter Passes the inputs it keeps and only
// Annotates the ones it drops.
func (x *Exec) Written() []graphmodel.Node {
	out := slices.Collect(maps.Values(x.written))
	slices.SortFunc(out, func(a, b graphmodel.Node) int { return strings.Compare(a.ID, b.ID) })

	return out
}

// MaxWrittenIDs bounds the ids a scan records of what it wrote: the
// scan event travels the postgres relay, which truncates near 8 KB.
const MaxWrittenIDs = 100

// WrittenIDs is the ids of Written, the first MaxWrittenIDs.
func (x *Exec) WrittenIDs() []string {
	w := x.Written()
	out := make([]string, 0, min(len(w), MaxWrittenIDs))
	for _, n := range w[:min(len(w), MaxWrittenIDs)] {
		out = append(out, n.ID)
	}

	return out
}

// Counts reports how many node and edge writes the run made.
func (x *Exec) Counts() (nodes, edges int) { return x.nodes, x.edges }
