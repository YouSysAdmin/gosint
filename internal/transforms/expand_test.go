// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"github.com/yousysadmin/gosint/internal/core/ids"
	amodel "github.com/yousysadmin/gosint/internal/models/attachment"
	"slices"
	"testing"
	"time"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/expand"
)

// A person's fields unfold into phones, emails, bank accounts (held by
// the person), a social profile as a site or a username, and relatives.
// A nested address goes through its type, and a type with no such field
// is told so.
func TestFieldsToEntities(t *testing.T) {
	h := newHarness(t)
	jane := h.node("individual", map[string]any{
		"full_name":             "Jane Roe",
		"phone_numbers":         []any{"+33 6 12 34 56 78"},
		"email_addresses":       []any{"jane@example.com"},
		"bank_accounts":         []any{"FR7612345678901234567890123"},
		"social_media_profiles": []any{"https://github.com/janeroe", "@janeroe"},
		"spouse_name":           "John Roe",
		"current_address":       map[string]any{"address": "1 Main St", "city": "Paris", "country": "FR", "zip": "75001"},
	})
	card := h.node("creditcard", map[string]any{"card_number": "4111111111111111", "cardholder_name": "Jane Roe"})
	phrase := h.node("phrase", map[string]any{"text": "nothing here"})
	h.run(expand.FieldsToEntities{}, nil, jane, card, phrase)

	got := h.edges()
	for _, want := range []string{
		"Jane Roe -HAS_PHONE-> +33612345678",
		"Jane Roe -HAS_EMAIL-> jane@example.com",
		"Jane Roe -HAS_BANK_ACCOUNT-> ****0123",
		"Jane Roe -HAS_SOCIAL_ACCOUNT-> https://github.com/janeroe",
		"Jane Roe -HAS_SOCIAL_ACCOUNT-> janeroe",
		"Jane Roe -MARRIED_TO-> John Roe",
		"Jane Roe -LIVES_AT-> 1 Main St, Paris, FR",
		"Visa ****1111 -HELD_BY-> Jane Roe",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}

	nodes, _, _ := h.st.Graph.Graph(t.Context(), h.sketch)
	for _, n := range nodes {
		if n.Type == "bankaccount" && n.Properties["account_holder"] != "Jane Roe" {
			t.Errorf("bank account = %v", n.Properties)
		}
	}

	warned := 0
	for _, l := range h.log {
		if l == "warning: nothing here: a phrase has no field that names another entity" {
			warned++
		}
	}

	if warned != 1 {
		t.Errorf("log = %q", h.log)
	}
}

// What a run writes carries its origin: the scan, the transform and the
// input it was found from. Found again by a later run, the entity keeps
// its first origin and records the later one as seen, the edge keeps its
// origin too.
func TestWritesCarryTheirOrigin(t *testing.T) {
	h := newHarness(t)
	jane := h.node("individual", map[string]any{"full_name": "Jane Roe", "email_addresses": []any{"jane@example.com"}})
	h.run(expand.FieldsToEntities{}, nil, jane)

	h.scan = "scan-2"
	h.run(expand.FieldsToEntities{}, nil, jane)

	nodes, edges, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	var email graphmodel.Node
	for _, n := range nodes {
		if n.Type == "email" {
			email = n
		}
	}

	origin, _ := email.Metadata["origin"].(map[string]any)
	seen, _ := email.Metadata["seen"].(map[string]any)
	if origin["scan"] != "scan-1" || origin["transform"] != "fields_to_entities" || origin["from"] != "Jane Roe" || origin["from_id"] != jane.ID || origin["via"] != "transform" || origin["at"] == nil {
		t.Errorf("origin = %v", origin)
	}

	if seen["scan"] != "scan-2" {
		t.Errorf("seen = %v", seen)
	}

	if len(edges) != 1 {
		t.Fatalf("edges = %v", edges)
	}

	if o, _ := edges[0].Properties["origin"].(map[string]any); o["scan"] != "scan-1" || o["transform"] != "fields_to_entities" {
		t.Errorf("edge origin = %v", o)
	}
}

// The files attached to an entity give what they say about themselves:
// a photo's place and camera, a document's author and company.
func TestFilesToEntities(t *testing.T) {
	h := newHarness(t)
	photo := h.node("photo", map[string]any{"title": "Harbour"})
	doc := h.node("document", map[string]any{"title": "Plan"})
	now := time.Now().UTC()
	for _, f := range []amodel.Attachment{
		{ID: ids.New(), SketchID: h.sketch, NodeID: photo.ID, Name: "shot.jpg", ContentType: "image/jpeg", Hash: "h1", UploaderID: "u", CreatedAt: now,
			Metadata: map[string]any{"latitude": -33.86, "longitude": -70.5, "camera_make": "Canon", "camera_model": "EOS R6"}},
		{ID: ids.New(), SketchID: h.sketch, NodeID: doc.ID, Name: "plan.docx", ContentType: "application/x", Hash: "h2", UploaderID: "u", CreatedAt: now,
			Metadata: map[string]any{"author": "Jane Roe", "last_modified_by": "Jane Roe", "company": "Example Ltd"}},
		{ID: ids.New(), SketchID: h.sketch, NodeID: doc.ID, Name: "notes.txt", ContentType: "text/plain", Hash: "h3", UploaderID: "u", CreatedAt: now},
	} {
		if err := h.st.Attachment.Create(t.Context(), &f); err != nil {
			t.Fatal(err)
		}
	}

	h.run(expand.FilesToEntities{}, nil, photo, doc)
	h.expectEdges(
		"Harbour -TAKEN_AT-> -33.86000, -70.50000",
		"Harbour -TAKEN_WITH-> Canon EOS R6",
		"Plan -AUTHORED_BY-> Jane Roe",
		"Plan -AUTHORED_AT-> Example Ltd",
	)
}
