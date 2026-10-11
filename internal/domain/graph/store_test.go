// Gosint, Copyright (c) 2026 YouSysAdmin

package graph_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/store"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// fixture makes a user, an investigation and a sketch, through a store
// sealing with the test key.
func fixture(t *testing.T, db database.Database) (*store.Store, string) {
	t.Helper()

	st := backend.BindStore(db, testVault(t))
	ctx := t.Context()
	now := time.Now().UTC()

	u := &usermodel.User{ID: ids.New(), Email: ids.New() + "@example.com", PasswordHash: "x", Active: true,
		CreatedAt: now, UpdatedAt: now}
	if err := st.User.Create(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}

	inv := &invmodel.Investigation{ID: ids.New(), Name: "case", Status: "active", OwnerID: u.ID,
		CreatedAt: now, UpdatedAt: now}
	if err := st.Investigation.Create(ctx, inv); err != nil {
		t.Fatalf("create investigation: %v", err)
	}

	sk := &sketchmodel.Sketch{ID: ids.New(), InvestigationID: inv.ID, Title: "s", Status: "active", OwnerID: u.ID,
		CreatedAt: now, UpdatedAt: now}
	if err := st.Sketch.Create(ctx, sk); err != nil {
		t.Fatalf("create sketch: %v", err)
	}

	return st, sk.ID
}

func upsert(t *testing.T, st *store.Store, sketchID string, nodes ...graphmodel.Node) []graphmodel.Node {
	t.Helper()

	out, err := st.Graph.UpsertNodes(t.Context(), sketchID, nodes)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	return out
}

func edge(t *testing.T, st *store.Store, sketchID, from, to, label string) *graphmodel.Edge {
	t.Helper()

	e, err := st.Graph.UpsertEdge(t.Context(), sketchID, graphmodel.Edge{SourceID: from, TargetID: to, Label: label})
	if err != nil || e == nil {
		t.Fatalf("edge %s: %v %v", label, e, err)
	}

	return e
}

// Finding an entity again must add to it, never erase what an earlier
// transform found.
func TestUpsertMergesIntoTheSameNode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)

		first := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "example.com",
			Properties: map[string]any{"domain": "example.com", "whois": map[string]any{"registrar": "A"}}})[0]

		second := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "example.com",
			Properties: map[string]any{"root": true, "whois": map[string]any{"created": "2001"}}})[0]

		if first.ID != second.ID {
			t.Fatalf("same entity got two nodes: %s and %s", first.ID, second.ID)
		}

		whois, _ := second.Properties["whois"].(map[string]any)
		if second.Properties["domain"] != "example.com" || second.Properties["root"] != true ||
			whois["registrar"] != "A" || whois["created"] != "2001" {
			t.Errorf("properties not merged: %v", second.Properties)
		}

		nodes, _, err := st.Graph.Graph(t.Context(), sk)
		if err != nil {
			t.Fatal(err)
		}

		if len(nodes) != 1 || nodes[0].Version != 2 {
			t.Errorf("got %d nodes, version %d, want 1 node at version 2", len(nodes), nodes[0].Version)
		}
	})
}

func TestTheSameLabelOfAnotherTypeIsAnotherNode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)

		out := upsert(t, st, sk,
			graphmodel.Node{Type: "username", Label: "alice"},
			graphmodel.Node{Type: "individual", Label: "alice"})
		if out[0].ID == out[1].ID {
			t.Error("two types shared one node")
		}
	})
}

func TestEdgesAreUniqueAndNeedBothEndsOnTheSketch(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		_, other := fixture(t, db)
		n := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a"}, graphmodel.Node{Type: "ip", Label: "1.1.1.1"})
		foreign := upsert(t, st, other, graphmodel.Node{Type: "ip", Label: "2.2.2.2"})[0]

		a := edge(t, st, sk, n[0].ID, n[1].ID, "RESOLVES_TO")
		b := edge(t, st, sk, n[0].ID, n[1].ID, "RESOLVES_TO")
		if a.ID != b.ID {
			t.Error("the same relationship got two edges")
		}

		e, err := st.Graph.UpsertEdge(t.Context(), sk, graphmodel.Edge{SourceID: n[0].ID, TargetID: foreign.ID, Label: "X"})
		if err != nil || e != nil {
			t.Errorf("edge to another sketch's node = %v, %v, want nil", e, err)
		}
	})
}

func TestDeletingANodeTakesItsEdges(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		n := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a"}, graphmodel.Node{Type: "ip", Label: "1.1.1.1"})
		edge(t, st, sk, n[0].ID, n[1].ID, "RESOLVES_TO")

		count, err := st.Graph.DeleteNodes(t.Context(), sk, []string{n[1].ID})
		if err != nil || count != 1 {
			t.Fatalf("delete = %d, %v", count, err)
		}

		nodes, edges, err := st.Graph.Graph(t.Context(), sk)
		if err != nil {
			t.Fatal(err)
		}

		if len(nodes) != 1 || len(edges) != 0 {
			t.Errorf("after delete: %d nodes, %d edges, want 1 and 0", len(nodes), len(edges))
		}
	})
}

// Merging keeps the edges to the outside, with their own labels, and
// drops the ones among the merged nodes.
func TestMergeMovesOutsideEdgesOntoTheKeptNode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		n := upsert(t, st, sk,
			graphmodel.Node{Type: "individual", Label: "J. Doe", Properties: map[string]any{"a": 1}},
			graphmodel.Node{Type: "individual", Label: "John Doe", Properties: map[string]any{"b": 2}},
			graphmodel.Node{Type: "email", Label: "j@example.com"},
			graphmodel.Node{Type: "phone", Label: "+100"})
		keep, drop, mail, phone := n[0], n[1], n[2], n[3]

		edge(t, st, sk, drop.ID, mail.ID, "HAS_EMAIL")
		edge(t, st, sk, phone.ID, drop.ID, "BELONGS_TO")
		edge(t, st, sk, keep.ID, drop.ID, "ALIAS_OF")

		got, err := st.Graph.MergeNodes(t.Context(), sk, []string{keep.ID, drop.ID}, keep.ID,
			graphmodel.Node{Label: "John Doe Merged"})
		if err != nil || got == nil {
			t.Fatalf("merge = %v, %v", got, err)
		}

		if got.ID != keep.ID || got.Label != "John Doe Merged" || got.Properties["a"] == nil || got.Properties["b"] == nil {
			t.Errorf("merged node = %+v", got)
		}

		nodes, edges, err := st.Graph.Graph(t.Context(), sk)
		if err != nil {
			t.Fatal(err)
		}

		if len(nodes) != 3 {
			t.Errorf("%d nodes after merge, want 3", len(nodes))
		}

		want := map[string]bool{
			keep.ID + ">" + mail.ID + ":HAS_EMAIL":   true,
			phone.ID + ">" + keep.ID + ":BELONGS_TO": true,
		}

		if len(edges) != len(want) {
			t.Fatalf("%d edges after merge, want %d: %+v", len(edges), len(want), edges)
		}

		for _, e := range edges {
			if !want[e.SourceID+">"+e.TargetID+":"+e.Label] {
				t.Errorf("unexpected edge %s -%s-> %s", e.SourceID, e.Label, e.TargetID)
			}
		}
	})
}

func TestNeighborsIsOneHop(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		n := upsert(t, st, sk,
			graphmodel.Node{Type: "domain", Label: "a"},
			graphmodel.Node{Type: "ip", Label: "1"},
			graphmodel.Node{Type: "asn", Label: "AS1"})
		edge(t, st, sk, n[0].ID, n[1].ID, "RESOLVES_TO")
		edge(t, st, sk, n[1].ID, n[2].ID, "BELONGS_TO")

		nodes, edges, err := st.Graph.Neighbors(t.Context(), sk, n[0].ID)
		if err != nil {
			t.Fatal(err)
		}

		if len(nodes) != 2 || len(edges) != 1 {
			t.Errorf("neighbors of a: %d nodes, %d edges, want 2 and 1", len(nodes), len(edges))
		}
	})
}

func TestPatchAndPositions(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		n := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a", Properties: map[string]any{"k": "v"}})[0]
		ctx := t.Context()

		got, err := st.Graph.UpdateNode(ctx, sk, n.ID, graphmodel.NodePatch{
			Color: new("#f00"), Properties: map[string]any{"k2": "v2"}})
		if err != nil || got == nil {
			t.Fatalf("patch = %v, %v", got, err)
		}

		if got.Color != "#f00" || got.Properties["k"] != "v" || got.Properties["k2"] != "v2" {
			t.Errorf("patched node = %+v", got)
		}

		count, err := st.Graph.SetPositions(ctx, sk, []graphmodel.Position{{NodeID: n.ID, X: 10, Y: -4.5}, {NodeID: ids.New(), X: 1}})
		if err != nil || count != 1 {
			t.Fatalf("positions = %d, %v", count, err)
		}

		back, err := st.Graph.Node(ctx, sk, n.ID)
		if err != nil || back.X == nil || *back.X != 10 || *back.Y != -4.5 {
			t.Errorf("position not stored: %+v %v", back, err)
		}
	})
}

// A sketch going takes its whole graph with it.
func TestDeletingTheInvestigationCascades(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a"})
		ctx := t.Context()

		s, err := st.Sketch.Get(ctx, sk)
		if err != nil {
			t.Fatal(err)
		}

		if err := st.Investigation.Delete(ctx, s.InvestigationID); err != nil {
			t.Fatal(err)
		}

		if s, _ := st.Sketch.Get(ctx, sk); s != nil {
			t.Error("sketch survived its investigation")
		}

		nodes, _, err := st.Graph.Graph(ctx, sk)
		if err != nil || len(nodes) != 0 {
			t.Errorf("graph after delete: %d nodes, %v", len(nodes), err)
		}
	})
}

// Deleting an organization with its dependents takes what was found
// through it, whichever way the edges point, and keeps the domain it
// was found from and an IP that is also reached another way.
func TestDependentsAreWhatHangsOnlyFromTheDeletedNode(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		add := func(typ, label string) string {
			return upsert(t, st, sk, graphmodel.Node{Type: typ, Label: label})[0].ID
		}

		root := add("domain", "acme.io")
		other := add("domain", "other.io")
		shared := add("ip", "1.1.1.1")
		org := add("organization", "Acme")
		boss := add("individual", "Jane Roe")
		home := add("location", "1 Main St")
		mail := add("email", "jane@acme.io")
		office := add("ip", "2.2.2.2")

		edge(t, st, sk, root, org, "RELATED_TO")
		edge(t, st, sk, other, shared, "RESOLVES_TO")
		edge(t, st, sk, org, shared, "HAS_IP")
		edge(t, st, sk, boss, org, "WORKS_FOR")
		edge(t, st, sk, boss, home, "LIVES_AT")
		edge(t, st, sk, boss, mail, "HAS_EMAIL")
		edge(t, st, sk, org, office, "HAS_IP")

		got, err := st.Graph.Dependents(t.Context(), sk, []string{org})
		if err != nil {
			t.Fatal(err)
		}

		want := []string{boss, home, mail, office}
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("dependents = %v, want %v (boss, home, mail, office)", got, want)
		}
	})
}

// A piece holding an entity older than the deleted node existed before
// it, so it was not found through it and stays.
func TestAnOlderEntityKeepsItsPiece(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		n := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "parent.io"})
		mid := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "mid.io"})
		leaf := upsert(t, st, sk, graphmodel.Node{Type: "ip", Label: "3.3.3.3"})
		edge(t, st, sk, n[0].ID, mid[0].ID, "HAS_SUBDOMAIN")
		edge(t, st, sk, mid[0].ID, leaf[0].ID, "RESOLVES_TO")

		got, err := st.Graph.Dependents(t.Context(), sk, []string{mid[0].ID})
		if err != nil {
			t.Fatal(err)
		}

		if !slices.Equal(got, []string{leaf[0].ID}) {
			t.Errorf("dependents of mid.io = %v, want only its IP", got)
		}
	})
}

// A node's origin is written once: a later upsert carrying another is
// merged without it, the rest of its metadata lands.
func TestTheFirstOriginStays(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		first := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a.io",
			Metadata: map[string]any{"origin": map[string]any{"scan": "one"}}})
		again := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a.io",
			Metadata: map[string]any{"origin": map[string]any{"scan": "two"}, "seen": map[string]any{"scan": "two"}}})
		if first[0].ID != again[0].ID {
			t.Fatal("the upsert made a second node")
		}

		origin, _ := again[0].Metadata["origin"].(map[string]any)
		seen, _ := again[0].Metadata["seen"].(map[string]any)
		if origin["scan"] != "one" || seen["scan"] != "two" {
			t.Errorf("metadata after the second upsert = %v", again[0].Metadata)
		}

		// Without an origin yet, the first one to come lands.
		bare := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "b.io"})
		late := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "b.io",
			Metadata: map[string]any{"origin": map[string]any{"scan": "late"}}})
		if o, _ := late[0].Metadata["origin"].(map[string]any); bare[0].ID != late[0].ID || o["scan"] != "late" {
			t.Errorf("a late origin on a bare node = %v", late[0].Metadata)
		}
	})
}

// Revert writes a snapshot back: a deleted node returns under its own
// id with its edge, a changed one gets its old label, an added one
// goes. The journal holds secret fields sealed, never in clear.
func TestRevertWritesASnapshotBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		a := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a.io"})[0]
		b := upsert(t, st, sk, graphmodel.Node{Type: "ip", Label: "1.1.1.1"})[0]
		ab := edge(t, st, sk, a.ID, b.ID, "RESOLVES_TO")
		before := graphmodel.Snapshot{Nodes: []graphmodel.Node{a, b}, Edges: []graphmodel.Edge{*ab}}

		// Delete b, rename a, add c, as an edit would.
		if _, err := st.Graph.DeleteNodes(t.Context(), sk, []string{b.ID}); err != nil {
			t.Fatal(err)
		}

		label := "renamed.io"
		if _, err := st.Graph.UpdateNode(t.Context(), sk, a.ID, graphmodel.NodePatch{Label: &label, Key: &label}); err != nil {
			t.Fatal(err)
		}

		c := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "c.io"})[0]
		after := graphmodel.Snapshot{Nodes: []graphmodel.Node{{ID: a.ID, Type: "domain", Label: label, Key: label}, c}}

		if err := st.Graph.Revert(t.Context(), sk, after, before); err != nil {
			t.Fatal(err)
		}

		nodes, edges, err := st.Graph.Graph(t.Context(), sk)
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]string{}
		for _, n := range nodes {
			got[n.ID] = n.Label
		}

		if len(got) != 2 || got[a.ID] != "a.io" || got[b.ID] != "1.1.1.1" {
			t.Errorf("after the revert: %v", got)
		}

		if len(edges) != 1 || edges[0].ID != ab.ID || edges[0].Label != "RESOLVES_TO" {
			t.Errorf("edges after the revert: %+v", edges)
		}

		// A secret in a journaled node is sealed in the row, like in the
		// node's own, through a store bound to a vault.
		v, err := vault.New(make([]byte, 32))
		if err != nil {
			t.Fatal(err)
		}

		sealed := backend.BindStore(db, v)
		cred := upsert(t, sealed, sk, graphmodel.Node{Type: "credential", Label: "root@box",
			Properties: map[string]any{"username": "root", "service": "box", "password": "hunter2-plain"}})[0]
		owner, err := sealed.Sketch.Get(t.Context(), sk)
		if err != nil || owner == nil {
			t.Fatalf("sketch: %v %v", owner, err)
		}

		ch := &graphmodel.Change{SketchID: sk, UserID: owner.OwnerID, Kind: "add", After: graphmodel.Snapshot{Nodes: []graphmodel.Node{cred}}}
		if err := sealed.Graph.RecordChange(t.Context(), ch); err != nil {
			t.Fatal(err)
		}

		base := database.NewBase(db)

		var raw string
		if err := base.QueryRow(t.Context(), `SELECT after_state FROM graph_changes WHERE id = ?`, ch.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}

		if strings.Contains(raw, "hunter2-plain") {
			t.Error("the journal holds the password in clear")
		}

		if next, _ := sealed.Graph.NextUndo(t.Context(), sk, ch.UserID); next == nil || next.Kind != "add" {
			t.Errorf("NextUndo = %v", next)
		}
	})
}

// A revert whose old key another node took since leaves that node as it
// is and still writes the rest back, so one later find never blocks the
// undo stack.
func TestRevertSkipsAKeyTakenSince(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		a := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a.io"})[0]
		b := upsert(t, st, sk, graphmodel.Node{Type: "ip", Label: "1.1.1.1"})[0]
		ab := edge(t, st, sk, a.ID, b.ID, "RESOLVES_TO")
		before := graphmodel.Snapshot{Nodes: []graphmodel.Node{a, b}, Edges: []graphmodel.Edge{*ab}}

		label := "renamed.io"
		if _, err := st.Graph.UpdateNode(t.Context(), sk, a.ID, graphmodel.NodePatch{Label: &label, Key: &label}); err != nil {
			t.Fatal(err)
		}

		// The edge's id now names another relationship, and a.io is back
		// as another node.
		if _, err := st.Graph.DeleteEdges(t.Context(), sk, []string{ab.ID}); err != nil {
			t.Fatal(err)
		}

		again := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "a.io"})[0]
		base := database.NewBase(db)
		v := testVault(t)
		idx, _ := v.Index("gosint/edge-label", "OTHER")
		sealedLabel, _ := v.SealText(database.AAD("edges", "label_sealed", ab.ID), "OTHER")
		props, _ := v.SealText(database.AAD("edges", "properties", ab.ID), "{}")
		if _, err := base.Exec(t.Context(), `INSERT INTO edges (id, sketch_id, source_id, target_id, label, label_sealed, properties, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, ab.ID, sk, b.ID, again.ID, idx, sealedLabel, props, ab.CreatedAt, ab.CreatedAt); err != nil {
			t.Fatal(err)
		}

		after := graphmodel.Snapshot{Nodes: []graphmodel.Node{{ID: a.ID, Type: "domain", Label: label, Key: label}, b}}
		if err := st.Graph.Revert(t.Context(), sk, after, before); err != nil {
			t.Fatalf("revert: %v", err)
		}

		nodes, edges, err := st.Graph.Graph(t.Context(), sk)
		if err != nil {
			t.Fatal(err)
		}

		got := map[string]string{}
		for _, n := range nodes {
			got[n.ID] = n.Label
		}

		if got[a.ID] != label || got[again.ID] != "a.io" || got[b.ID] != "1.1.1.1" {
			t.Errorf("after the revert: %v", got)
		}

		labels := map[string]bool{}
		for _, e := range edges {
			labels[e.Label] = true
			if e.Label == "RESOLVES_TO" && e.ID == ab.ID {
				t.Error("the restored edge took the id of another relationship")
			}
		}

		if !labels["RESOLVES_TO"] || !labels["OTHER"] {
			t.Errorf("edges after the revert: %+v", edges)
		}
	})
}

// A search treats % and _ as text and takes a trailing backslash, which
// postgres refuses as an unescaped pattern.
func TestSearchEscapesThePattern(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		upsert(t, st, sk, graphmodel.Node{Type: "username", Label: "jane_doe"},
			graphmodel.Node{Type: "username", Label: "janexdoe"})

		owner, err := st.Sketch.Get(t.Context(), sk)
		if err != nil || owner == nil {
			t.Fatalf("sketch: %v %v", owner, err)
		}

		for q, want := range map[string]int{"e_d": 1, "%": 0, `doe\`: 0, "doe": 2} {
			hits, err := st.Graph.Search(t.Context(), owner.OwnerID, store.SearchQuery{Q: q})
			if err != nil {
				t.Fatalf("search %q: %v", q, err)
			}

			if len(hits) != want {
				t.Errorf("search %q found %d, want %d", q, len(hits), want)
			}
		}
	})
}

// An import counts the relationships it made, not the ones it found
// already there.
func TestAnImportCountsNewEdgesOnly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		st, sk := fixture(t, db)
		nodes := []graphmodel.Node{{Type: "domain", Label: "a.io"}, {Type: "ip", Label: "1.1.1.1"}}
		edges := []graphmodel.EdgeRef{{Source: 0, Target: 1, Label: "RESOLVES_TO"}}

		for i, want := range []int{1, 0} {
			_, created, err := st.Graph.Import(t.Context(), sk, nodes, edges)
			if err != nil {
				t.Fatal(err)
			}

			if created != want {
				t.Errorf("import %d created %d edges, want %d", i+1, created, want)
			}
		}
	})
}
