// Gosint, Copyright (c) 2026 YouSysAdmin

package graph_test

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/vault"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/backend"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/store"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// testVault seals with a fixed test key, so every store of a test opens
// what another wrote.
func testVault(t *testing.T) *vault.Vault {
	t.Helper()

	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	return v
}

func sealedStore(t *testing.T, db database.Database) (*store.Store, *vault.Vault) {
	t.Helper()

	v := testVault(t)

	return backend.BindStore(db, v), v
}

// Every column a node or an edge holds about the entity is sealed in
// the row and opens through the store, a credential's password included.
func TestTheGraphIsSealedAtRest(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		_, sk := fixture(t, db)
		st, _ := sealedStore(t, db)

		n := upsert(t, st, sk, graphmodel.Node{Type: "credential", Key: "root@hidden-box", Label: "root@hidden-box",
			Properties: map[string]any{"username": "root", "service": "hidden-box", "password": "hunter2"},
			Metadata:   map[string]any{"notes": "met-at-dawn"}, Style: graphmodel.Style{Image: "https://pics.example/face.png"}})[0]
		d := upsert(t, st, sk, graphmodel.Node{Type: "domain", Label: "secret-example.org"})[0]
		if _, err := st.Graph.UpsertEdge(t.Context(), sk, graphmodel.Edge{SourceID: n.ID, TargetID: d.ID, Label: "USED_ON",
			Properties: map[string]any{"seen": "edge-secret"}}); err != nil {
			t.Fatal(err)
		}

		base := database.NewBase(db)
		var key, keySealed, label, props, meta, image string
		if err := base.QueryRow(t.Context(), `SELECT node_key, key_sealed, node_label, properties, metadata, image FROM nodes WHERE id = ?`, n.ID).
			Scan(&key, &keySealed, &label, &props, &meta, &image); err != nil {
			t.Fatal(err)
		}

		var edgeProps string
		if err := base.QueryRow(t.Context(), `SELECT properties FROM edges WHERE sketch_id = ?`, sk).Scan(&edgeProps); err != nil {
			t.Fatal(err)
		}

		for _, raw := range []string{key, keySealed, label, props, meta, image, edgeProps} {
			for _, clear := range []string{"hidden-box", "hunter2", "met-at-dawn", "pics.example", "edge-secret"} {
				if strings.Contains(raw, clear) {
					t.Errorf("%q is in clear in %s", clear, raw)
				}
			}
		}

		got, err := st.Graph.Node(t.Context(), sk, n.ID)
		if err != nil || got == nil {
			t.Fatal(got, err)
		}

		if got.Key != "root@hidden-box" || got.Label != "root@hidden-box" || got.Properties["password"] != "hunter2" ||
			got.Metadata["notes"] != "met-at-dawn" || got.Image != "https://pics.example/face.png" {
			t.Errorf("read back = %+v", got)
		}

		if again := upsert(t, st, sk, graphmodel.Node{Type: "credential", Key: "root@hidden-box", Label: "root@hidden-box"}); again[0].ID != n.ID {
			t.Error("the same key made a second node")
		}

		edges, err := st.Graph.EdgesTouching(t.Context(), sk, []string{n.ID})
		if err != nil || len(edges) != 1 || edges[0].Properties["seen"] != "edge-secret" {
			t.Errorf("edges = %+v %v", edges, err)
		}
	})
}

// A piece of a label is found through its hashed pieces, any case, and
// a near miss sharing pieces is not.
func TestSearchThroughGrams(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		plain, sk := fixture(t, db)
		st, _ := sealedStore(t, db)
		owner, err := plain.Sketch.Get(t.Context(), sk)
		if err != nil || owner == nil {
			t.Fatal(owner, err)
		}

		upsert(t, st, sk,
			graphmodel.Node{Type: "domain", Label: "Mail.Example.com"},
			graphmodel.Node{Type: "domain", Label: "example.org"},
			graphmodel.Node{Type: "domain", Label: "plexam.net"})

		search := func(q string) []string {
			hits, err := st.Graph.Search(t.Context(), owner.OwnerID, store.SearchQuery{Q: q})
			if err != nil {
				t.Fatal(err)
			}

			var out []string
			for _, h := range hits {
				out = append(out, h.Label)
			}

			return out
		}

		if got := search("EXAMPLE"); len(got) != 2 || got[0] != "Mail.Example.com" || got[1] != "example.org" {
			t.Errorf("EXAMPLE = %v", got)
		}

		if got := search("il"); len(got) != 1 {
			t.Errorf("il = %v", got)
		}

		// "examp" shares pieces with plexam.net only partly.
		if got := search("examp"); len(got) != 2 {
			t.Errorf("examp = %v", got)
		}

		if got := search("nothing-here"); len(got) != 0 {
			t.Errorf("nothing-here = %v", got)
		}

		if got := search("e"); len(got) != 3 {
			t.Errorf("e = %v", got)
		}

		hits, err := st.Graph.Search(t.Context(), owner.OwnerID, store.SearchQuery{Type: "domain", Key: "example.org"})
		if err != nil || len(hits) != 1 {
			t.Errorf("by key = %v %v", hits, err)
		}
	})
}
