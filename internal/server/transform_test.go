// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"context"
	"encoding/json/v2"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/env"
	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/transformdir"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/worker"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	flowdomain "github.com/yousysadmin/gosint/internal/domain/flow"
	"github.com/yousysadmin/gosint/internal/domain/transformtemplate"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
)

// startWorker runs a worker with the transform handler on rt until the
// test ends.
func startWorker(t *testing.T, rt *env.Runtime) {
	t.Helper()

	w := worker.New(worker.Config{
		Scans: rt.Store.Scan, Logs: rt.Store.Log, Bus: rt.Events, Log: slog.New(slog.DiscardHandler),
		NodeID: "test",
		Tune: func() worker.Tuning {
			return worker.Tuning{Enabled: true, Concurrency: 2, Poll: 20 * time.Millisecond, Lease: time.Second, MaxAttempts: 2}
		},
	})
	runner := transform.RunnerConfig{
		Lookup: transform.ByName(rt.Transforms), Types: types.Builtin(), Graph: rt.Store.Graph,
		Secrets: func(ctx context.Context, owner, ref string) (string, bool, error) {
			return vaultkey.Secret(ctx, rt, owner, ref)
		},
		HTTP: http.DefaultClient, Web: http.DefaultClient, Resolver: net.DefaultResolver,
	}
	w.Handle(transform.ScanKind, transform.Handler(runner))
	w.Handle(coreflow.ScanKind, coreflow.Handler(coreflow.RunnerConfig{
		RunnerConfig: runner, Registry: rt.Transforms, Definition: flowdomain.Definition(rt),
	}))
	runner.Lookup = transformtemplate.Lookup(rt)
	w.Handle(transformtemplate.ScanKind, transform.Handler(runner))
	rt.WakeWorker = w.Wake

	ctx, stop := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	t.Cleanup(func() { stop(); <-done })
}

func TestLaunchingATransform(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		startWorker(t, rt)

		alice := signUp(t, srv, "alice@example.com")
		viewer := signUp(t, srv, "viewer@example.com")
		base, _ := sketchFor(t, alice)

		var email, ip struct {
			Node idBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "email", "label": "john@example.com"}, &email)
		alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "1.1.1.1"}, &ip)

		var list struct {
			Transforms []struct {
				Name string `json:"name"`
			} `json:"transforms"`
		}

		alice.expect(http.StatusOK, "GET", "/api/transforms?type=email", nil, &list)
		if len(list.Transforms) == 0 {
			t.Fatal("no transforms for email")
		}

		alice.expect(http.StatusNotFound, "POST", base+"/transforms/no_such/launch", map[string]any{"node_ids": []string{email.Node.ID}})
		alice.expect(http.StatusBadRequest, "POST", base+"/transforms/email_to_domain/launch", map[string]any{"node_ids": []string{ip.Node.ID}})
		alice.expect(http.StatusBadRequest, "POST", base+"/transforms/email_to_domain/launch",
			map[string]any{"node_ids": []string{email.Node.ID}, "params": map[string]string{"typo": "x"}})

		var refused struct {
			Fields []struct {
				Field string `json:"field"`
			} `json:"fields"`
		}

		if code := alice.do("POST", base+"/transforms/email_to_breaches/launch", map[string]any{"node_ids": []string{email.Node.ID}}, &refused); code != http.StatusBadRequest ||
			len(refused.Fields) != 1 || refused.Fields[0].Field != "params.HIBP_API_KEY" {
			t.Errorf("leaks without a key = %d %+v", code, refused.Fields)
		}

		// With the secret stored the launch goes through, and the scan
		// row carries the parameter names only: every member of the
		// sketch can read scans, the key is the owner's. The API URL
		// points at a closed port so the run fails fast and offline.
		alice.expect(http.StatusCreated, "POST", "/api/secrets", map[string]string{"name": "HIBP_API_KEY", "value": "s3cret-hibp"})

		var withKey map[string]any
		alice.expect(http.StatusCreated, "POST", base+"/transforms/email_to_breaches/launch",
			map[string]any{"node_ids": []string{email.Node.ID}, "params": map[string]string{"HIBP_API_URL": "http://127.0.0.1:9/"}}, &withKey)

		var listed map[string]any
		alice.expect(http.StatusOK, "GET", base+"/scans", nil, &listed)
		for name, v := range map[string]any{"launch": withKey, "list": listed} {
			if raw, _ := json.Marshal(v); strings.Contains(string(raw), "s3cret") {
				t.Errorf("the %s answer carries the vault value: %s", name, raw)
			}
		}

		var launched struct {
			Scan struct {
				ID      string         `json:"id"`
				Input   map[string]any `json:"input"`
				Status  string         `json:"status"`
				Details map[string]any `json:"details"`
			} `json:"scan"`
		}

		alice.expect(http.StatusCreated, "POST", base+"/transforms/email_to_domain/launch",
			map[string]any{"node_ids": []string{email.Node.ID, ip.Node.ID}}, &launched)

		awaitScan(t, alice, base+"/scans/"+launched.Scan.ID, &launched)

		var g graphBody
		alice.expect(http.StatusOK, "GET", base+"/graph", nil, &g)
		if len(g.Edges) != 1 || g.Edges[0].Label != "HAS_DOMAIN" {
			t.Errorf("graph after the scan = %+v", g)
		}

		// The scan names what it wrote, the counts alone say only how much.
		if ids, _ := launched.Scan.Details["written_ids"].([]any); len(ids) != 1 {
			t.Errorf("details = %v, want one written id", launched.Scan.Details)
		}

		// A viewer of someone else's sketch cannot even see it.
		viewer.expect(http.StatusNotFound, "POST", base+"/transforms/email_to_domain/launch",
			map[string]any{"node_ids": []string{email.Node.ID}})
	})
}

// A transform read from the transforms directory lists with its file,
// launches by name like a built-in one, and its scan writes what the
// script printed.
func TestAFileTransformRunsLikeABuiltInOne(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"echo.yaml": "name: domain_to_echo\ncategory: Domain\ninput: domain\noutput: ip\nrun: ./echo.sh\n",
		"echo.sh":   "#!/bin/sh\nread -r doc\necho '{\"type\": \"ip\", \"label\": \"203.0.113.9\", \"relation\": \"RESOLVES_TO\"}'\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o700); err != nil { //nolint:gosec // a test script must run
			t.Fatal(err)
		}
	}

	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	files := transformdir.New(dir, types.Builtin(), rt.Log)
	rt.Transforms.SetSource(files.Transforms)
	startWorker(t, rt)

	alice := signUp(t, srv, "alice@example.com")
	base, _ := sketchFor(t, alice)

	var list struct {
		Transforms []struct {
			Name string `json:"name"`
			File string `json:"file"`
		} `json:"transforms"`
	}

	alice.expect(http.StatusOK, "GET", "/api/transforms?type=domain", nil, &list)
	i := slices.IndexFunc(list.Transforms, func(e struct {
		Name string `json:"name"`
		File string `json:"file"`
	}) bool {
		return e.Name == "domain_to_echo"
	})
	if i < 0 || list.Transforms[i].File != "echo.yaml" {
		t.Fatalf("the catalog lacks the file transform: %+v", list.Transforms)
	}

	var domain struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &domain)

	var launched struct {
		Scan struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"scan"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/transforms/domain_to_echo/launch",
		map[string]any{"node_ids": []string{domain.Node.ID}}, &launched)
	awaitScan(t, alice, base+"/scans/"+launched.Scan.ID, &launched)

	var g graphBody
	alice.expect(http.StatusOK, "GET", base+"/graph", nil, &g)
	if len(g.Edges) != 1 || g.Edges[0].Label != "RESOLVES_TO" {
		t.Errorf("graph after the scan = %+v", g)
	}
}
