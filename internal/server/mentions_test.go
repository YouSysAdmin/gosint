// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// The analysis editor's mentions search one investigation's sketches:
// nothing from another investigation of the same account, a non-member
// gets the 404 a missing id gets, and an underscore is a character.
func TestMentionsSearchOneInvestigation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		alice := signUp(t, srv, "alice@example.com")
		bob := signUp(t, srv, "bob@example.com")

		mine, mineID := sketchFor(t, alice)
		elsewhere, _ := sketchFor(t, alice)
		inv := investigationOf(t, alice, mine)

		for _, name := range []string{"a_case", "abcase"} {
			alice.expect(http.StatusCreated, "POST", mine+"/nodes", map[string]any{"type": "username", "label": name})
		}

		alice.expect(http.StatusCreated, "POST", elsewhere+"/nodes", map[string]any{"type": "username", "label": "a_other"})

		find := func(q string) []string {
			var out struct {
				Hits []struct {
					Label       string `json:"label"`
					Type        string `json:"type"`
					SketchID    string `json:"sketch_id"`
					SketchTitle string `json:"sketch_title"`
				} `json:"hits"`
			}

			alice.expect(http.StatusOK, "GET", inv+"/entities?q="+q, nil, &out)

			labels := []string{}
			for _, h := range out.Hits {
				if h.SketchID != mineID || h.SketchTitle == "" || h.Type != "username" {
					t.Errorf("hit %+v is not from this investigation's sketch", h)
				}

				labels = append(labels, h.Label)
			}

			return labels
		}

		if got := find("a_"); !slices.Equal(got, []string{"a_case"}) {
			t.Errorf("a_ = %v, want a_case only", got)
		}

		if got := find(""); !slices.Equal(got, []string{"a_case", "abcase"}) {
			t.Errorf("empty = %v, want this investigation's entities by label", got)
		}

		bob.expect(http.StatusNotFound, "GET", inv+"/entities?q=a", nil)
	})
}
