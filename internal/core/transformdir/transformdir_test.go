// Gosint, Copyright (c) 2026 YouSysAdmin

package transformdir_test

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/transformdir"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/store"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

type lines []string

func (l *lines) Log(level logmodel.Level, message string, _ map[string]any) {
	*l = append(*l, string(level)+": "+message)
}

// sketch is a fresh sketch on a throwaway sqlite database.
type sketch struct {
	t   *testing.T
	st  *store.Store
	id  string
	log lines
}

func newSketch(t *testing.T) *sketch {
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

	return &sketch{t: t, st: st, id: sk.ID}
}

func (s *sketch) node(typ string, props map[string]any) graphmodel.Node {
	s.t.Helper()

	label, err := types.Builtin().Build(typ, props)
	if err != nil {
		s.t.Fatal(err)
	}

	out, err := s.st.Graph.UpsertNodes(s.t.Context(), s.id, []graphmodel.Node{{Type: typ, Key: types.Builtin().Key(typ, props), Label: label, Properties: props}})
	if err != nil {
		s.t.Fatal(err)
	}

	return out[0]
}

func (s *sketch) run(e transform.Transform, params map[string]string, inputs ...graphmodel.Node) *transform.Exec {
	s.t.Helper()

	x := transform.NewExec(transform.ExecConfig{SketchID: s.id, Graph: s.st.Graph, Types: types.Builtin(), Log: &s.log,
		Params: params, HTTP: http.DefaultClient, Web: http.DefaultClient})
	if err := e.Run(s.t.Context(), x, inputs); err != nil {
		s.t.Fatalf("%s: %v", e.Meta().Name, err)
	}

	return x
}

func (s *sketch) edges() []string {
	s.t.Helper()

	nodes, edges, err := s.st.Graph.Graph(s.t.Context(), s.id)
	if err != nil {
		s.t.Fatal(err)
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

// write puts a file in dir, dated so a rescan sees it as changed.
func write(t *testing.T, dir, name, content string) {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil { //nolint:gosec // a test script must run
		t.Fatal(err)
	}

	if err := os.Chtimes(path, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func newDir(t *testing.T) (*transformdir.Dir, string) {
	t.Helper()

	dir := t.TempDir()
	d := transformdir.New(dir, types.Builtin(), slog.New(slog.DiscardHandler))
	d.Rescan = time.Nanosecond
	d.Reserved = []string{"domain_to_ip"}

	return d, dir
}

func names(list []transform.Transform) []string {
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.Meta().Name)
	}

	return out
}

const manifest = `
name: domain_to_things
description: Prints what it was given
category: Domain
input: domain
output: ip
run: ./things.sh
timeout: 5
params:
  - name: suffix
    type: string
    default: ".x"
`

// The script reads its input and params on stdin and prints entities,
// an update and a log line, each becoming graph writes. The server's
// own environment stays out of it.
const script = `#!/bin/sh
read -r doc
label=$(printf '%s' "$doc" | sed -n 's/.*"label":"\([^"]*\)".*/\1/p')
suffix=$(printf '%s' "$doc" | sed -n 's/.*"suffix":"\([^"]*\)".*/\1/p')
echo "{\"type\": \"ip\", \"label\": \"203.0.113.9\", \"relation\": \"RESOLVES_TO\"}"
echo "{\"type\": \"domain\", \"properties\": {\"domain\": \"up$suffix\"}, \"relation\": \"ALIAS_OF\", \"reverse\": true}"
echo "{\"update\": {\"alive\": true}}"
echo "{\"log\": \"saw $label with GOSINT_VAULT_MASTER_KEY=${GOSINT_VAULT_MASTER_KEY:-unset} AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-unset} PATH=${PATH:+set}\"}"
echo "{\"type\": \"ip\", \"label\": \"not an ip\"}"
echo "not json"
echo "{\"type\": \"ip\", \"label\": \"203.0.113.10\", \"relation\": \"bad label\"}"
`

func TestAScriptWritesWhatItPrints(t *testing.T) {
	t.Setenv("GOSINT_VAULT_MASTER_KEY", "secret")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	d, dir := newDir(t)
	write(t, dir, "things.yaml", manifest)
	write(t, dir, "things.sh", script)

	list := d.Transforms()
	if got := names(list); !slices.Equal(got, []string{"domain_to_things"}) {
		t.Fatalf("loaded %q", got)
	}

	if f, ok := list[0].(interface{ File() string }); !ok || f.File() != "things.yaml" {
		t.Errorf("file = %v %v", f, ok)
	}

	s := newSketch(t)
	in := s.node("domain", map[string]any{"domain": "example.com"})
	x := s.run(list[0], map[string]string{"suffix": ".io"}, in)

	want := []string{"example.com -RESOLVES_TO-> 203.0.113.9", "up.io -ALIAS_OF-> example.com"}
	if got := s.edges(); !slices.Equal(got, want) {
		t.Errorf("edges %q, log %q", got, s.log)
	}

	got, err := s.st.Graph.Node(t.Context(), s.id, in.ID)
	if err != nil || got.Properties["alive"] != true {
		t.Errorf("the update did not land: %v %v", got, err)
	}

	if nodes, _ := x.Counts(); nodes != 3 {
		t.Errorf("counted %d nodes, want 3", nodes)
	}

	joined := strings.Join(s.log, "\n")
	for _, want := range []string{"saw example.com with GOSINT_VAULT_MASTER_KEY=unset AWS_SECRET_ACCESS_KEY=unset PATH=set",
		"not an ip", "a line is not JSON", "not UPPER_SNAKE_CASE"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log lacks %q:\n%s", want, joined)
		}
	}
}

func TestAFailingOrSlowScriptIsReportedAndTheRestGoesOn(t *testing.T) {
	d, dir := newDir(t)
	write(t, dir, "fail.yaml", "name: fails\ncategory: X\nrun: ./fail.sh\ntimeout: 1\n")
	write(t, dir, "fail.sh", "#!/bin/sh\nread -r doc\ncase \"$doc\" in *slow*) sleep 5;; esac\necho 'broken on purpose' >&2\nexit 3\n")

	list := d.Transforms()
	if len(list) != 1 {
		t.Fatalf("loaded %q", names(list))
	}

	s := newSketch(t)
	a := s.node("domain", map[string]any{"domain": "slow.example"})
	b := s.node("domain", map[string]any{"domain": "fast.example"})

	start := time.Now()
	s.run(list[0], nil, a, b)
	if time.Since(start) > 4*time.Second {
		t.Error("the slow script was not stopped at its timeout")
	}

	joined := strings.Join(s.log, "\n")
	if !strings.Contains(joined, "slow.example: stopped after 1s") || !strings.Contains(joined, "fast.example: exit status 3: broken on purpose") {
		t.Errorf("log:\n%s", joined)
	}
}

// A line over the cap ends the script at once and says why, instead of
// leaving it blocked on the pipe until the timeout.
func TestAnOverlongLineEndsTheScript(t *testing.T) {
	d, dir := newDir(t)
	write(t, dir, "long.yaml", "name: long\ncategory: X\nrun: ./long.sh\ntimeout: 30\n")
	write(t, dir, "long.sh", "#!/bin/sh\nread -r doc\nwhile :; do printf 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'; done\n")

	list := d.Transforms()
	if len(list) != 1 {
		t.Fatalf("loaded %q", names(list))
	}

	s := newSketch(t)
	start := time.Now()
	s.run(list[0], nil, s.node("domain", map[string]any{"domain": "a.example"}))
	if time.Since(start) > 10*time.Second {
		t.Error("the script ran on to its timeout")
	}

	if joined := strings.Join(s.log, "\n"); !strings.Contains(joined, "a line is over 1024 KB") {
		t.Errorf("log:\n%s", joined)
	}
}

func TestTheDirectoryIsReadAgain(t *testing.T) {
	d, dir := newDir(t)
	if got := d.Transforms(); len(got) != 0 {
		t.Fatalf("an empty directory loaded %q", names(got))
	}

	write(t, dir, "a.yaml", "name: a_thing\ncategory: X\nrun: ./a.sh\n")
	write(t, dir, "broken.yaml", "name: broken\ncategory: X\nrun: ./x\ntimeout: 0.1\n")
	write(t, dir, "taken.yaml", "name: domain_to_ip\ncategory: X\nrun: ./x\n")
	write(t, dir, "twice.yaml", "name: a_thing\ncategory: X\nrun: ./b.sh\n")
	write(t, dir, "notes.txt", "name: ignored\n")
	write(t, dir, "tpl.yaml", `
name: github-user
category: Username
input:
  type: username
  key: value
request:
  url: "https://api.example/users/{{value}}"
response:
  map:
    username: login
output:
  type: socialaccount
`)

	if got := names(d.Transforms()); !slices.Equal(got, []string{"a_thing", "github-user"}) {
		t.Errorf("loaded %q", got)
	}

	if err := os.Remove(filepath.Join(dir, "a.yaml")); err != nil {
		t.Fatal(err)
	}

	write(t, dir, "twice.yaml", "name: b_thing\ncategory: X\nrun: ./b.sh\n")
	if got := names(d.Transforms()); !slices.Equal(got, []string{"b_thing", "github-user"}) {
		t.Errorf("after the change %q", got)
	}

	// The registry serves them beside the built-in ones, which win.
	reg := transform.NewRegistry(transformdir.Script{M: &transformdir.Manifest{Name: "b_thing", Category: "Built"}})
	reg.SetSource(d.Transforms)
	if e, ok := reg.Get("github-user"); !ok || e.Meta().Input != "username" {
		t.Errorf("the registry lacks the file transform: %v %v", e, ok)
	}

	if e, _ := reg.Get("b_thing"); e.Meta().Category != "Built" {
		t.Errorf("the file transform shadowed the built-in: %+v", e.Meta())
	}

	if got := names(reg.For("")); !slices.Equal(got, []string{"b_thing", "github-user"}) {
		t.Errorf("For lists %q", got)
	}
}

func TestAManifestIsChecked(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"name: Bad Name\ncategory: X\nrun: ./x\n", "name must be"},
		{"name: ok\nrun: ./x\n", "category is required"},
		{"name: ok\ncategory: X\nrun: ./x\ninput: nothing\n", "not a known entity type"},
		{"name: ok\ncategory: X\nrun: ./x\nparams:\n  - name: pick\n    type: select\n", "select with no options"},
		{"name: ok\ncategory: X\nrun: ./x\nparams:\n  - name: a\n  - name: a\n", "declared twice"},
		{"name: ok\ncategory: X\nrun: ./x\nextra: 1\n", "field extra not found"},
	} {
		_, err := transformdir.ParseManifest([]byte(tc.src), types.Builtin())
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: err %v, want %q", tc.src, err, tc.want)
		}
	}

	m, err := transformdir.ParseManifest([]byte("name: ok\ncategory: X\nrun: python3 x.py\nparams:\n  - name: KEY\n    type: vaultSecret\n"), types.Builtin())
	if err != nil || m.Timeout != 60 || m.Params[0].Type != transform.ParamSecret {
		t.Errorf("a good manifest: %+v, %v", m, err)
	}
}
