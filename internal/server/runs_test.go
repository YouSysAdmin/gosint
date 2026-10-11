// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	logmodel "github.com/yousysadmin/gosint/internal/models/logentry"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// sketchFor makes an investigation and a sketch owned by a, returning
// the sketch's API path.
func sketchFor(t *testing.T, a *api) (string, string) {
	t.Helper()

	var inv struct {
		Investigation idBody `json:"investigation"`
	}

	a.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)

	var sk struct {
		Sketch idBody `json:"sketch"`
	}

	a.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/sketches",
		map[string]string{"title": "Map"}, &sk)

	return "/api/sketches/" + sk.Sketch.ID, sk.Sketch.ID
}

// awaitScan polls the scan until it settles and fails the test unless it
// completed.
func awaitScan(t *testing.T, a *api, path string, launched any) {
	t.Helper()

	status := func() string {
		raw, _ := json.Marshal(launched)

		var v struct {
			Scan struct {
				Status string `json:"status"`
			} `json:"scan"`
		}

		_ = json.Unmarshal(raw, &v)

		return v.Scan.Status
	}

	deadline := time.Now().Add(10 * time.Second)
	for s := status(); s != "completed" && s != "failed" && time.Now().Before(deadline); s = status() {
		time.Sleep(50 * time.Millisecond)
		a.expect(http.StatusOK, "GET", path, nil, launched)
	}

	if s := status(); s != "completed" {
		t.Fatalf("scan %s = %s", path, s)
	}
}

// A stored key is never answered back, opens only for its owner, and
// storing the same name again replaces it.
func TestVaultKeys(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		alice := signUp(t, srv, "alice@example.com")
		bob := signUp(t, srv, "bob@example.com")

		var put struct {
			Key map[string]any `json:"key"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/secrets", map[string]string{"name": "WHOXY_API_KEY", "value": "first"}, &put)
		if _, leaked := put.Key["value"]; leaked || put.Key["name"] != "WHOXY_API_KEY" {
			t.Errorf("stored key answered as %v", put.Key)
		}

		firstID := put.Key["id"]
		alice.expect(http.StatusCreated, "POST", "/api/secrets", map[string]string{"name": "WHOXY_API_KEY", "value": "second"}, &put)
		if put.Key["id"] != firstID {
			t.Error("storing the same name made a second key")
		}

		alice.expect(http.StatusBadRequest, "POST", "/api/secrets", map[string]string{"name": "has spaces", "value": "x"})

		var me struct {
			User idBody `json:"user"`
		}

		alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)

		got, ok, err := vaultkey.Secret(t.Context(), rt, me.User.ID, "WHOXY_API_KEY")
		if err != nil || !ok || got != "second" {
			t.Errorf("secret = %q, %v, %v", got, ok, err)
		}

		var list struct {
			Keys []map[string]any `json:"keys"`
		}

		bob.expect(http.StatusOK, "GET", "/api/secrets", nil, &list)
		if len(list.Keys) != 0 {
			t.Errorf("bob sees alice's keys: %v", list.Keys)
		}

		bob.expect(http.StatusNotFound, "DELETE", "/api/secrets/"+firstID.(string), nil)
		alice.expect(http.StatusNoContent, "DELETE", "/api/secrets/"+firstID.(string), nil)
	})
}

func TestScansOfASketch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		alice := signUp(t, srv, "alice@example.com")
		base, sketchID := sketchFor(t, alice)
		other, _ := sketchFor(t, alice)

		sc := &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, Kind: "transform", Name: "domain_to_ip"}
		if err := scan.Enqueue(t.Context(), rt, sc); err != nil {
			t.Fatal(err)
		}

		var list struct {
			Scans []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"scans"`
		}

		alice.expect(http.StatusOK, "GET", base+"/scans", nil, &list)
		if len(list.Scans) != 1 || list.Scans[0].Status != "pending" {
			t.Fatalf("scans = %+v", list.Scans)
		}

		// A scan is reached only through its own sketch.
		alice.expect(http.StatusNotFound, "GET", other+"/scans/"+sc.ID, nil)

		alice.expect(http.StatusConflict, "DELETE", base+"/scans/"+sc.ID, nil)
		alice.expect(http.StatusOK, "POST", base+"/scans/"+sc.ID+"/cancel", nil)
		alice.expect(http.StatusConflict, "POST", base+"/scans/"+sc.ID+"/cancel", nil)
		alice.expect(http.StatusNoContent, "DELETE", base+"/scans/"+sc.ID, nil)
		alice.expect(http.StatusNotFound, "GET", base+"/scans/"+sc.ID, nil)
	})
}

func TestTheLogOfASketch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		alice := signUp(t, srv, "alice@example.com")
		stranger := signUp(t, srv, "stranger@example.com")
		base, sketchID := sketchFor(t, alice)
		start := time.Now().UTC()

		for i, msg := range []string{"one", "two", "three"} {
			e := &logmodel.Entry{ID: ids.New(), SketchID: sketchID, Level: logmodel.LevelInfo, Message: msg,
				CreatedAt: start.Add(time.Duration(i) * time.Millisecond)}
			if err := rt.Store.Log.Append(t.Context(), e); err != nil {
				t.Fatal(err)
			}
		}

		var logs struct {
			Logs []struct {
				Message string `json:"message"`
			} `json:"logs"`
		}

		alice.expect(http.StatusOK, "GET", base+"/logs?limit=2", nil, &logs)
		if len(logs.Logs) != 2 || logs.Logs[0].Message != "two" || logs.Logs[1].Message != "three" {
			t.Errorf("newest two, oldest first = %+v", logs.Logs)
		}

		alice.expect(http.StatusBadRequest, "GET", base+"/logs?since=yesterday", nil)
		stranger.expect(http.StatusNotFound, "GET", base+"/logs", nil)

		var cleared struct {
			Count int `json:"count"`
		}

		alice.expect(http.StatusOK, "DELETE", base+"/logs", nil, &cleared)
		if cleared.Count != 3 {
			t.Errorf("cleared %d, want 3", cleared.Count)
		}
	})
}

// The stream needs a real listener: it never ends, and app.Test waits
// for the body to finish.
func TestTheEventStreamDeliversSketchEvents(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	stranger := signUp(t, srv, "stranger@example.com")
	base, sketchID := sketchFor(t, alice)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	go func() { _ = srv.App().Listener(ln, fiber.ListenConfig{DisableStartupMessage: true}) }()

	t.Cleanup(func() { _ = srv.Shutdown(time.Second) })

	open := func(cookie string) *http.Response {
		req, _ := http.NewRequestWithContext(t.Context(), "GET", "http://"+ln.Addr().String()+base+"/events", nil)
		req.Header.Set("Cookie", auth.CookieName+"="+cookie)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = resp.Body.Close() })

		return resp
	}

	//nolint:bodyclose // open closes it on cleanup
	if resp := open(stranger.cookie); resp.StatusCode != http.StatusNotFound {
		t.Errorf("stranger's stream = %d, want 404", resp.StatusCode)
	}

	resp := open(alice.cookie) //nolint:bodyclose // open closes it on cleanup

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}

	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}

		close(lines)
	}()

	next := func(prefix string) string {
		t.Helper()

		timeout := time.After(5 * time.Second)
		for {
			select {
			case l, ok := <-lines:
				if !ok {
					t.Fatal("stream ended")
				}

				if strings.HasPrefix(l, prefix) {
					return l
				}
			case <-timeout:
				t.Fatalf("no %q line in time", prefix)
			}
		}
	}

	if got := next("event:"); got != "event: connected" {
		t.Fatalf("first event = %q", got)
	}

	rt.Events.Publish(eventbus.Event{Type: eventbus.TypeLog, SketchID: sketchID,
		Data: logmodel.Entry{Message: "hello from a worker"}})

	if got := next("event:"); got != "event: log" {
		t.Fatalf("event = %q, want log", got)
	}

	if data := next("data:"); !strings.Contains(data, "hello from a worker") {
		t.Errorf("data = %q", data)
	}
}

// Wait hands back a scan once it settled, hearing of it through the
// bus, and finds one that settled before it looked through the poll.
func TestWaitReturnsASettledScan(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	startWorker(t, rt)
	alice := signUp(t, srv, "alice@example.com")
	base, sketchID := sketchFor(t, alice)

	var email struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"}, &email)

	var me struct {
		User idBody `json:"user"`
	}

	alice.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
	sc := &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, OwnerID: me.User.ID, Kind: "transform", Name: "email_to_domain",
		Input: map[string]any{"node_ids": []any{email.Node.ID}, "params": map[string]any{}}}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done, err := scan.Wait(ctx, rt, sc, func() error { return scan.Enqueue(ctx, rt, sc) }, nil)
	if err != nil || done.Status != scanmodel.StatusCompleted || done.Details["nodes"] != 1.0 {
		t.Fatalf("Wait = %+v, %v", done, err)
	}

	// Already settled: nothing to queue, the first look finds it.
	again, err := scan.Wait(ctx, rt, sc, func() error { return nil }, nil)
	if err != nil || again.Status != scanmodel.StatusCompleted {
		t.Errorf("Wait on a settled scan = %+v, %v", again, err)
	}
}
