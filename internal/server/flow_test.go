// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"encoding/json/v2"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/domain/schedule"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

func TestRunningAFlow(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		startWorker(t, rt)
		alice := signUp(t, srv, "alice@example.com")
		bob := signUp(t, srv, "bob@example.com")

		def := map[string]any{"input": "email", "steps": []map[string]any{
			{"id": "domain", "transform": "email_to_domain", "after": "input"},
			{"id": "root", "transform": "domain_to_root_domain", "after": "domain"},
			{"id": "keep", "transform": "domain_root_only", "after": "root"},
			{"id": "user", "transform": "email_to_username", "after": "input"},
		}}

		alice.expect(http.StatusBadRequest, "POST", "/api/flows", map[string]any{"name": "bad", "definition": map[string]any{
			"input": "email", "steps": []map[string]any{{"id": "x", "transform": "ip_to_asn", "after": "input"}}}})

		var planned struct {
			Steps []struct {
				ID string `json:"id"`
			} `json:"steps"`
		}

		alice.expect(http.StatusOK, "POST", "/api/flows/check", map[string]any{"definition": def}, &planned)
		if len(planned.Steps) != 4 {
			t.Errorf("planned = %+v", planned.Steps)
		}

		var created struct {
			Flow idBody `json:"flow"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/flows", map[string]any{"name": "email recon", "definition": def}, &created)
		bob.expect(http.StatusNotFound, "GET", "/api/flows/"+created.Flow.ID, nil)

		sketch, _ := sketchFor(t, alice)

		var email struct {
			Node idBody `json:"node"`
		}

		alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "email", "label": "jane@mail.example.co.uk"}, &email)

		var launched struct {
			Scan struct {
				ID      string         `json:"id"`
				Status  string         `json:"status"`
				Details map[string]any `json:"details"`
			} `json:"scan"`
		}

		alice.expect(http.StatusCreated, "POST", sketch+"/flows/"+created.Flow.ID+"/launch",
			map[string]any{"node_ids": []string{email.Node.ID}}, &launched)

		awaitScan(t, alice, sketch+"/scans/"+launched.Scan.ID, &launched)

		// The filter step took the root domain its parent wrote and passed
		// it on, and the scan says which entities the flow wrote.
		steps, _ := launched.Scan.Details["steps"].(map[string]any)
		keep, _ := steps["keep"].(map[string]any)
		if keep["inputs"] != 1.0 || keep["passed"] != 1.0 || keep["nodes"] != 0.0 {
			t.Errorf("filter step = %v, want 1 input kept and passed on, none written", steps["keep"])
		}

		if ids, _ := launched.Scan.Details["written_ids"].([]any); len(ids) == 0 {
			t.Errorf("details = %v, want written_ids", launched.Scan.Details)
		}

		var g graphBody
		alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)

		labels := map[string]string{}
		for _, n := range g.Nodes {
			labels[n.ID] = n.Label
		}

		var edges []string
		for _, e := range g.Edges {
			edges = append(edges, labels[e.Source]+" -"+e.Label+"-> "+labels[e.Target])
		}

		for _, want := range []string{
			"jane@mail.example.co.uk -HAS_DOMAIN-> mail.example.co.uk",
			"example.co.uk -HAS_SUBDOMAIN-> mail.example.co.uk",
			"jane@mail.example.co.uk -HAS_USERNAME-> jane",
		} {
			if !slices.Contains(edges, want) {
				t.Errorf("missing %q in %q", want, edges)
			}
		}
	})
}

// A flow exports as a document with no id and imports as the caller's
// own, one file holding several. A flow naming a transform this install
// lacks, or a name the caller has already, is skipped and said so.
func TestFlowsMoveAsFiles(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	def := map[string]any{"input": "domain", "steps": []map[string]any{{"id": "s1", "after": "input", "transform": "domain_to_ip"}}}

	var created struct {
		Flow idBody `json:"flow"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/flows", map[string]any{"name": "Resolve", "definition": def}, &created)

	var one map[string]any
	alice.expect(http.StatusOK, "GET", "/api/flows/"+created.Flow.ID+"/export", nil, &one)
	flow, _ := one["flow"].(map[string]any)
	if one["gosint"] != "flow" || flow["name"] != "Resolve" || flow["id"] != nil {
		t.Fatalf("export = %v", one)
	}

	var all map[string]any
	alice.expect(http.StatusOK, "GET", "/api/flows/export", nil, &all)
	if flows, _ := all["flows"].([]any); all["gosint"] != "flows" || len(flows) != 1 {
		t.Fatalf("export all = %v", all)
	}

	// Bob imports the file with a second flow naming a transform that
	// does not exist, and a third one: two land, one is skipped.
	doc := map[string]any{"gosint": "flows", "version": 1, "flows": []map[string]any{
		{"name": "Resolve", "definition": def},
		{"name": "Broken", "definition": map[string]any{"input": "domain", "steps": []map[string]any{{"id": "s1", "after": "input", "transform": "no_such"}}}},
		{"name": "Again", "description": "d", "definition": def},
	}}
	raw, _ := json.Marshal(doc)

	var result struct {
		Imported int      `json:"imported"`
		Skipped  []string `json:"skipped"`
	}

	body, ct := multipartFile(t, "flows.json", raw)
	if code := bob.do("POST", "/api/flows/import", body, &result, "Content-Type", ct); code != http.StatusOK {
		t.Fatalf("import = %d", code)
	}

	if result.Imported != 2 || len(result.Skipped) != 1 || !strings.Contains(result.Skipped[0], "Broken") {
		t.Errorf("import = %+v", result)
	}

	// The same file again: both names taken now, nothing imported.
	body, ct = multipartFile(t, "flows.json", raw)
	if code := bob.do("POST", "/api/flows/import", body, nil, "Content-Type", ct); code != http.StatusBadRequest {
		t.Errorf("a second import = %d, want 400", code)
	}
}

// multipartFile is a multipart body holding one file.
func multipartFile(t *testing.T, name string, content []byte) ([]byte, string) {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}

	_, _ = part.Write(content)
	_ = mw.Close()

	return body.Bytes(), mw.FormDataContentType()
}

// A schedule runs a flow on the sketch every so often: run now queues
// it on every entity of the input type, the list sums the run up off
// the provenance, the tick queues a due one once (a second claim with
// the same version loses), and a flow that is gone switches it off.
func TestAScheduledFlowRunsAgain(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	startWorker(t, rt)
	alice := signUp(t, srv, "alice@example.com")
	sketch, sketchID := sketchFor(t, alice)

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"})

	var flow struct {
		Flow idBody `json:"flow"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/flows", map[string]any{"name": "Mail domains",
		"definition": map[string]any{"input": "email", "steps": []map[string]any{{"id": "s1", "after": "input", "transform": "email_to_domain"}}}}, &flow)

	var created struct {
		Schedule struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		} `json:"schedule"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/schedules", map[string]any{"flow_id": flow.Flow.ID, "every_seconds": 3600}, &created)
	alice.expect(http.StatusBadRequest, "POST", sketch+"/schedules", map[string]any{"flow_id": flow.Flow.ID, "every_seconds": 10})

	var ran struct {
		Scan struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"scan"`
	}

	// It runs with alice's secrets: an editor neither changes, runs nor
	// removes it, a member who manages the investigation may.
	bob := signUp(t, srv, "bob@example.com")
	carol := signUp(t, srv, "carol@example.com")
	inv := investigationOf(t, alice, sketch)
	alice.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "bob@example.com", "role": "editor"})
	alice.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "carol@example.com", "role": "admin"})

	one := sketch + "/schedules/" + created.Schedule.ID
	bob.expect(http.StatusForbidden, "PATCH", one, map[string]any{"every_seconds": 300})
	bob.expect(http.StatusForbidden, "POST", one+"/run", nil)
	bob.expect(http.StatusForbidden, "DELETE", one, nil)
	carol.expect(http.StatusOK, "PATCH", one, map[string]any{"every_seconds": 3600})

	alice.expect(http.StatusCreated, "POST", sketch+"/schedules/"+created.Schedule.ID+"/run", nil, &ran)
	if ran.Scan.Name != "Mail domains (scheduled)" {
		t.Errorf("scan = %+v", ran.Scan)
	}

	awaitScan(t, alice, sketch+"/scans/"+ran.Scan.ID, &ran)

	var list struct {
		Schedules []struct {
			ID         string `json:"id"`
			LastScanID string `json:"last_scan_id"`
			Summary    *struct {
				Status string `json:"status"`
				New    int    `json:"new"`
			} `json:"summary"`
		} `json:"schedules"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/schedules", nil, &list)
	if len(list.Schedules) != 1 || list.Schedules[0].LastScanID != ran.Scan.ID || list.Schedules[0].Summary == nil ||
		list.Schedules[0].Summary.New != 1 || list.Schedules[0].Summary.Status != "completed" {
		t.Errorf("list = %+v", list.Schedules)
	}

	// Just run, the tick leaves it alone. Dated back, it runs again, and
	// a stale claim after that loses.
	if err := schedule.Tick(t.Context(), rt); err != nil {
		t.Fatal(err)
	}

	var scans struct {
		Scans []struct {
			ID string `json:"id"`
		} `json:"scans"`
	}

	alice.expect(http.StatusOK, "GET", sketch+"/scans", nil, &scans)
	if len(scans.Scans) != 1 {
		t.Fatalf("the tick ran a schedule that was not due: %d scans", len(scans.Scans))
	}

	s, err := rt.Store.Schedule.Get(t.Context(), sketchID, created.Schedule.ID)
	if err != nil || s == nil {
		t.Fatal(err)
	}

	stale := *s
	base := database.NewBase(rt.DB)
	if _, err := base.Exec(t.Context(), `UPDATE schedules SET last_run_at = ? WHERE id = ?`, time.Now().UTC().Add(-2*time.Hour), s.ID); err != nil {
		t.Fatal(err)
	}

	if err := schedule.Tick(t.Context(), rt); err != nil {
		t.Fatal(err)
	}

	alice.expect(http.StatusOK, "GET", sketch+"/scans", nil, &scans)
	if len(scans.Scans) != 2 {
		t.Errorf("the due schedule queued %d scans in all, want 2", len(scans.Scans))
	}

	if ok, _ := rt.Store.Schedule.Claim(t.Context(), &stale, "x", time.Now().UTC()); ok {
		t.Error("a stale claim went through")
	}

	// The flow deleted, the next run switches the schedule off.
	alice.expect(http.StatusNoContent, "DELETE", "/api/flows/"+flow.Flow.ID, nil)
	alice.expect(http.StatusBadRequest, "POST", sketch+"/schedules/"+created.Schedule.ID+"/run", nil)
	alice.expect(http.StatusOK, "GET", sketch+"/schedules", nil, &list)
	s, _ = rt.Store.Schedule.Get(t.Context(), sketchID, created.Schedule.ID)
	if s.Enabled || s.LastError == "" {
		t.Errorf("after the flow went: %+v", s)
	}

	alice.expect(http.StatusNoContent, "DELETE", sketch+"/schedules/"+created.Schedule.ID, nil)
}

// A schedule runs as its owner, with their secrets: once the owner can
// no longer edit the investigation, it stops instead of going on.
func TestAScheduleStopsWithItsOwnersAccess(t *testing.T) {
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")
	sketch, sketchID := sketchFor(t, alice)
	inv := investigationOf(t, alice, sketch)
	alice.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "bob@example.com", "role": "editor"})
	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"})

	var flow struct {
		Flow idBody `json:"flow"`
	}

	bob.expect(http.StatusCreated, "POST", "/api/flows", map[string]any{"name": "Mail domains",
		"definition": map[string]any{"input": "email", "steps": []map[string]any{{"id": "s1", "after": "input", "transform": "email_to_domain"}}}}, &flow)

	var created struct {
		Schedule idBody `json:"schedule"`
	}

	bob.expect(http.StatusCreated, "POST", sketch+"/schedules", map[string]any{"flow_id": flow.Flow.ID, "every_seconds": 3600}, &created)

	var me struct {
		User idBody `json:"user"`
	}

	bob.expect(http.StatusOK, "GET", "/api/auth/me", nil, &me)
	alice.expect(http.StatusOK, "PUT", inv+"/members/"+me.User.ID, map[string]string{"role": "viewer"})

	s, err := rt.Store.Schedule.Get(t.Context(), sketchID, created.Schedule.ID)
	if err != nil || s == nil {
		t.Fatal(s, err)
	}

	if sc, err := schedule.Run(t.Context(), rt, s, time.Now().UTC()); err == nil || sc != nil {
		t.Fatalf("a viewer's schedule ran: %v %v", sc, err)
	}

	s, _ = rt.Store.Schedule.Get(t.Context(), sketchID, created.Schedule.ID)
	if s.Enabled || s.LastError == "" {
		t.Errorf("the schedule is still on: %+v", s)
	}
}
