// Gosint, Copyright (c) 2026 YouSysAdmin

package transformdir_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/sandbox"
	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// TestMain lets this test binary act as the sandbox helper, since a
// confined script is started through the running binary.
func TestMain(m *testing.M) {
	sandbox.Main()

	os.Exit(m.Run())
}

func (s *sketch) runWith(e transform.Transform, tools tool.Settings, inputs ...graphmodel.Node) error {
	s.t.Helper()

	x := transform.NewExec(transform.ExecConfig{SketchID: s.id, Graph: s.st.Graph, Types: types.Builtin(), Log: &s.log,
		HTTP: http.DefaultClient, Web: http.DefaultClient, Tools: tools})

	return e.Run(s.t.Context(), x, inputs)
}

// probe reports, one log line each, what the script could reach.
const probe = `#!/bin/sh
read -r doc
try() { if eval "$2" >/dev/null 2>&1; then echo "{\"log\": \"$1 allowed\"}"; else echo "{\"log\": \"$1 refused\"}"; fi; }
try secret 'cat %q'
try extra 'cat %q'
try parent 'cat /proc/$PPID/environ'
try scratch 'echo x > "$TMPDIR/scratch"'
try own 'echo x > ./own.txt'
try run 'cp /bin/sh "$TMPDIR/sh" && "$TMPDIR/sh" -c true'
`

// A confined script reads the system, its own directory and what the
// administrator lets through, writes only its scratch directory, and
// never the server's files or its environment, and runs nothing it
// wrote. A path this node lacks is said in the log.
func TestAConfinedScriptReachesOnlyWhatItIsGiven(t *testing.T) {
	if !sandbox.Supported() {
		t.Skip("no Landlock on this system")
	}

	secret := filepath.Join(t.TempDir(), "gosint.yaml")
	write(t, filepath.Dir(secret), "gosint.yaml", "vault:\n  unseal_key: top\n")

	extra := t.TempDir()
	write(t, extra, "wordlist.txt", "admin\n")

	d, dir := newDir(t)
	write(t, dir, "probe.yaml", "name: probe\ncategory: X\ninput: domain\nrun: ./probe.sh\n")
	write(t, dir, "probe.sh", fmt.Sprintf(probe, secret, filepath.Join(extra, "wordlist.txt")))

	s := newSketch(t)
	in := s.node("domain", map[string]any{"domain": "example.com"})

	tools := tool.Settings{tool.ScriptsName: {ReadPaths: []string{extra, "/nowhere/gosint-wordlists"}}}
	if err := s.runWith(d.Transforms()[0], tools, in); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(s.log, "\n")
	for _, want := range []string{"secret refused", "extra allowed", "parent refused", "scratch allowed", "own refused",
		"run refused", "/nowhere/gosint-wordlists is not on this node"} {
		if !strings.Contains(joined, want) {
			t.Errorf("log lacks %q:\n%s", want, joined)
		}
	}
}

// Switched on where it cannot work, the run is refused rather than
// left unconfined.
func TestASandboxThatCannotWorkRefusesToRun(t *testing.T) {
	if sandbox.Supported() {
		t.Skip("this system has Landlock")
	}

	d, dir := newDir(t)
	write(t, dir, "probe.yaml", "name: probe\ncategory: X\ninput: domain\nrun: ./probe.sh\n")
	write(t, dir, "probe.sh", "#!/bin/sh\necho '{\"log\": \"ran\"}'\n")

	s := newSketch(t)
	in := s.node("domain", map[string]any{"domain": "example.com"})

	on := true

	err := s.runWith(d.Transforms()[0], tool.Settings{tool.ScriptsName: {Sandbox: &on}}, in)
	if err == nil || !strings.Contains(err.Error(), "Landlock") {
		t.Fatalf("err = %v, want the sandbox refusal", err)
	}

	if strings.Contains(strings.Join(s.log, "\n"), "ran") {
		t.Error("the script ran unconfined")
	}
}
