// Gosint, Copyright (c) 2026 YouSysAdmin

package transformdir

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Manifest describes a script as a transform. The script runs once per
// input entity, in the directory, reading one JSON object on stdin
// ({"input": {id, type, label, properties}, "params": {...}}) and
// printing JSON lines: an entity ({"type", "label", "properties",
// "relation", "reverse"}) linked from the input, {"update": {...}}
// laying properties over the input, or {"log": "...", "level": "..."}.
type Manifest struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Category    string `yaml:"category"`
	Icon        string `yaml:"icon"`

	// Input is the entity type the script takes, "" for any. Output is
	// what it mostly writes, "" when mixed.
	Input  string `yaml:"input"`
	Output string `yaml:"output"`

	// Run is the command line, split on spaces, run in the directory:
	// "./find.sh" or "python3 find.py".
	Run string `yaml:"run"`

	// Timeout bounds one run of the script, in seconds, 60 by default.
	Timeout float64 `yaml:"timeout"`

	Params []transform.Param `yaml:"params"`
}

const (
	defaultTimeout = 60.0
	maxTimeout     = 3600.0

	// maxOutput caps what one run of the script may print.
	maxOutput = 16 << 20

	// maxLine caps one printed line.
	maxLine = 1 << 20

	// stderrTail is how much of the script's stderr is kept for the log.
	stderrTail = 2000
)

var (
	nameRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)
	paramRe    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	relationRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

	paramTypes = []transform.ParamType{transform.ParamString, transform.ParamURL, transform.ParamNumber,
		transform.ParamSelect, transform.ParamBool, transform.ParamSecret}
)

// ParseManifest reads and checks a manifest. Unknown keys are refused.
func ParseManifest(source []byte, reg *types.Registry) (*Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(source))
	dec.KnownFields(true)

	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: %w", err)
	}

	m.Name = strings.TrimSpace(m.Name)
	m.Input = strings.ToLower(strings.TrimSpace(m.Input))
	m.Output = strings.ToLower(strings.TrimSpace(m.Output))
	m.Run = strings.TrimSpace(m.Run)
	if m.Timeout == 0 {
		m.Timeout = defaultTimeout
	}

	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if !nameRe.MatchString(m.Name) {
		fail("name must be lowercase letters, digits, dash and underscore")
	}

	if strings.TrimSpace(m.Category) == "" {
		fail("category is required")
	}

	for _, typ := range []string{m.Input, m.Output} {
		if _, ok := reg.Get(typ); typ != "" && !ok {
			fail("type %q is not a known entity type", typ)
		}
	}

	if m.Run == "" {
		fail("run is required")
	}

	if m.Timeout < 1 || m.Timeout > maxTimeout {
		fail("timeout must be between 1 and %d seconds", int(maxTimeout))
	}

	seen := map[string]bool{}
	for i := range m.Params {
		p := &m.Params[i]
		if p.Type == "" {
			p.Type = transform.ParamString
		}

		switch {
		case !paramRe.MatchString(p.Name):
			fail("param %q is not a valid name", p.Name)
		case seen[p.Name]:
			fail("param %q is declared twice", p.Name)
		case !slices.Contains(paramTypes, p.Type):
			fail("param %q has an unknown type %q", p.Name, p.Type)
		case p.Type == transform.ParamSelect && len(p.Options) == 0:
			fail("param %q is a select with no options", p.Name)
		}

		seen[p.Name] = true
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	return &m, nil
}

// Script runs a manifest's command as a transform.
type Script struct {
	M *Manifest

	// Dir is the directory the script runs in.
	Dir string
}

// Meta describes the script as a transform.
func (s Script) Meta() transform.Meta {
	return transform.Meta{Name: s.M.Name, Category: s.M.Category, Description: s.M.Description, Icon: s.M.Icon,
		Input: s.M.Input, Output: s.M.Output, Params: slices.Clone(s.M.Params)}
}

// Run runs the script once per input. A failed run is logged and the
// next input goes on, a cancelled scan stops.
func (s Script) Run(ctx context.Context, x *transform.Exec, inputs []graphmodel.Node) error {
	for _, in := range inputs {
		if err := ctx.Err(); err != nil {
			return err
		}

		if err := s.one(ctx, x, in); err != nil {
			x.Warn("%s: %s: %v", s.M.Name, in.Label, err)
		}
	}

	return nil
}

// stdinDoc is what the script reads.
type stdinDoc struct {
	Input  stdinNode         `json:"input"`
	Params map[string]string `json:"params"`
}

type stdinNode struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
}

// line is one JSON line the script prints.
type line struct {
	Type       string         `json:"type"`
	Label      string         `json:"label"`
	Properties map[string]any `json:"properties"`
	Relation   string         `json:"relation"`
	Reverse    bool           `json:"reverse"`
	Update     map[string]any `json:"update"`
	Log        string         `json:"log"`
	Level      string         `json:"level"`
}

func (s Script) one(ctx context.Context, x *transform.Exec, in graphmodel.Node) error {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.M.Timeout*float64(time.Second)))
	defer cancel()

	args := strings.Fields(s.M.Run)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...) //nolint:gosec // the operator's own manifest
	cmd.Dir = s.Dir
	cmd.Env = scrubbedEnv()
	ownGroup(cmd)

	// A stopped script's children may hold the pipes open, this is how
	// long they are waited for.
	cmd.WaitDelay = 2 * time.Second

	params := map[string]string{}
	for _, p := range s.M.Params {
		params[p.Name] = x.Param(p.Name)
	}

	input, err := json.Marshal(stdinDoc{Input: stdinNode{ID: in.ID, Type: in.Type, Label: in.Label, Properties: in.Properties}, Params: params})
	if err != nil {
		return err
	}

	cmd.Stdin = bytes.NewReader(input)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	var stderr tail
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	// Reading stops early on a line or an output over the caps. The
	// script is ended then, or it would block on the full pipe until the
	// timeout and the reason would read as a timeout.
	out := &capped{r: stdout, left: maxOutput}
	readErr := s.read(ctx, x, in, out)
	if readErr == nil && out.full {
		readErr = fmt.Errorf("stopped reading after %d MB of output", maxOutput>>20)
	}

	if readErr != nil {
		cancel()
	}

	waitErr := cmd.Wait()
	if readErr != nil {
		return readErr
	}

	if waitErr != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("stopped after %.0fs: %w", s.M.Timeout, ctx.Err())
		}

		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", waitErr, lastLine(msg))
		}

		return waitErr
	}

	return nil
}

// capped reads up to left bytes and says when it was cut.
type capped struct {
	r    io.Reader
	left int64
	full bool
}

func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// One more byte says the output goes on past the cap.
		var b [1]byte
		if n, _ := c.r.Read(b[:]); n > 0 {
			c.full = true
		}

		return 0, io.EOF
	}

	if int64(len(p)) > c.left {
		p = p[:c.left]
	}

	n, err := c.r.Read(p)
	c.left -= int64(n)

	return n, err
}

// read handles the script's lines as they come.
func (s Script) read(ctx context.Context, x *transform.Exec, in graphmodel.Node, r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), maxLine)
	for sc.Scan() {
		raw := bytes.TrimSpace(sc.Bytes())
		if len(raw) == 0 {
			continue
		}

		var l line
		if err := json.Unmarshal(raw, &l); err != nil {
			x.Warn("%s: %s: a line is not JSON: %s", s.M.Name, in.Label, truncate(string(raw), 120))

			continue
		}

		if err := s.handle(ctx, x, in, l); err != nil {
			x.Warn("%s: %s: %v", s.M.Name, in.Label, err)
		}
	}

	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("a line is over %d KB", maxLine>>10)
		}

		return err
	}

	return nil
}

func (s Script) handle(ctx context.Context, x *transform.Exec, in graphmodel.Node, l line) error {
	switch {
	case l.Log != "":
		if l.Level == "warning" || l.Level == "warn" || l.Level == "error" {
			x.Warn("%s: %s", s.M.Name, l.Log)
		} else {
			x.Info("%s: %s", s.M.Name, l.Log)
		}

		return nil
	case l.Update != nil:
		props := maps.Clone(in.Properties)
		if props == nil {
			props = map[string]any{}
		}

		maps.Copy(props, l.Update)
		if _, err := x.Node(ctx, in.Type, props); err != nil {
			return err
		}

		x.Found("%s: enriched %s", s.M.Name, in.Label)

		return nil
	case l.Type != "":
		props := maps.Clone(l.Properties)
		if props == nil {
			props = map[string]any{}
		}

		if l.Label != "" {
			props["label"] = l.Label
		}

		relation := strings.TrimSpace(l.Relation)
		if relation == "" {
			relation = "RELATED_TO"
		}

		if !relationRe.MatchString(relation) {
			return fmt.Errorf("relation %q is not UPPER_SNAKE_CASE", relation)
		}

		if !l.Reverse {
			_, err := x.NodeAndLink(ctx, in, relation, strings.ToLower(l.Type), props)

			return err
		}

		n, err := x.NodeFrom(ctx, in, strings.ToLower(l.Type), props)
		if err != nil {
			return err
		}

		if err := x.Link(ctx, n, in, relation); err != nil {
			return err
		}

		x.Found("%s -> %s -> %s", n.Label, relation, in.Label)

		return nil
	default:
		return errors.New("a line names no type, update or log")
	}
}

// scrubbedEnv is the server's environment without its own settings, so
// a script never sees the database DSN, the master key, a bucket's
// credentials or an API key of the server's. Only what a process needs
// to run passes: the path, the home, the locale, the temp dir and the
// zone.
func scrubbedEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if passes(name) {
			out = append(out, kv)
		}
	}

	return out
}

// passedEnv is what a script inherits from the server's environment,
// the Windows names included since a process there needs them to start.
var passedEnv = []string{"PATH", "HOME", "USER", "SHELL", "LANG", "LANGUAGE", "TMPDIR", "TMP", "TEMP", "TZ",
	"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "USERPROFILE", "APPDATA", "LOCALAPPDATA"}

// passes says a variable reaches the script. Names are compared without
// case, as Windows does.
func passes(name string) bool {
	if len(name) > 3 && strings.EqualFold(name[:3], "LC_") {
		return true
	}

	return slices.ContainsFunc(passedEnv, func(p string) bool { return strings.EqualFold(p, name) })
}

// tail keeps the last stderrTail bytes written.
type tail struct {
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	if len(t.buf) > stderrTail {
		t.buf = t.buf[len(t.buf)-stderrTail:]
	}

	return len(p), nil
}

func (t *tail) String() string { return string(t.buf) }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")

	return truncate(strings.TrimSpace(lines[len(lines)-1]), 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
