// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// A hand edit, an undo and a move each tell the sketch's live stream
// who changed the graph, so the other members reload.
func TestHandEditsReachTheLiveStream(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, sketchID := sketchFor(t, alice)

	sub, err := rt.Events.Subscribe(sketchID)
	if err != nil {
		t.Fatal(err)
	}

	defer sub.Close()

	next := func(want string) eventbus.GraphChange {
		t.Helper()
		for {
			select {
			case e := <-sub.C:
				if e.Type != eventbus.TypeGraph {
					continue
				}

				g, _ := e.Data.(eventbus.GraphChange)
				if g.Kind != want {
					t.Fatalf("graph event %q, want %q", g.Kind, want)
				}

				return g
			case <-time.After(3 * time.Second):
				t.Fatalf("no graph event %q", want)
			}
		}
	}

	var n struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com"}, &n)
	if g := next("add example.com"); g.Name != "alice@example.com" || g.By == "" {
		t.Errorf("event = %+v", g)
	}

	alice.expect(http.StatusOK, "PUT", sketch+"/nodes/positions", map[string]any{"positions": []map[string]any{{"node_id": n.Node.ID, "x": 1, "y": 2}}})
	next("moved")

	alice.expect(http.StatusOK, "POST", sketch+"/undo", nil)
	next("undo add example.com")
}
