// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// An analysis belongs to the investigation: editors write, viewers only
// read, the list leaves the content out, and a document that is not
// JSON is refused.
func TestAnalysesFollowTheInvestigationsRoles(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	owner := signUp(t, srv, "owner@example.com")
	viewer := signUp(t, srv, "viewer@example.com")

	var created struct {
		Investigation idBody `json:"investigation"`
	}

	owner.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &created)
	inv := "/api/investigations/" + created.Investigation.ID
	owner.expect(http.StatusCreated, "POST", inv+"/members", map[string]string{"email": "viewer@example.com", "role": "viewer"})

	var a struct {
		Analysis struct {
			ID      string         `json:"id"`
			Title   string         `json:"title"`
			Content map[string]any `json:"content"`
		} `json:"analysis"`
	}

	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "paragraph"}}}
	owner.expect(http.StatusCreated, "POST", inv+"/analyses", map[string]any{"title": "Findings", "content": doc}, &a)
	if a.Analysis.Content["type"] != "doc" {
		t.Errorf("content came back as %v", a.Analysis.Content)
	}

	one := inv + "/analyses/" + a.Analysis.ID
	viewer.expect(http.StatusOK, "GET", one, nil)
	viewer.expect(http.StatusForbidden, "PUT", one, map[string]any{"title": "Mine now"})
	viewer.expect(http.StatusForbidden, "DELETE", one, nil)

	code := owner.do("PUT", one, map[string]any{"title": "Findings", "content": "not a document"}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("a string as content = %d", code)
	}

	owner.expect(http.StatusOK, "PUT", one, map[string]any{"title": "Findings, day 2"}, &a)
	if a.Analysis.Title != "Findings, day 2" || a.Analysis.Content["type"] != "doc" {
		t.Errorf("after the title-only update: %+v", a.Analysis)
	}

	var list struct {
		Analyses []struct {
			Title   string         `json:"title"`
			Content map[string]any `json:"content"`
		} `json:"analyses"`
	}

	viewer.expect(http.StatusOK, "GET", inv+"/analyses", nil, &list)
	if len(list.Analyses) != 1 || len(list.Analyses[0].Content) != 0 {
		t.Errorf("list = %+v, want one entry without content", list.Analyses)
	}

	owner.expect(http.StatusNoContent, "DELETE", one, nil)
	code, body, _ := owner.raw("GET", one, nil)
	if code != http.StatusNotFound || !strings.Contains(body, "not found") {
		t.Errorf("after delete = %d %s", code, body)
	}
}
