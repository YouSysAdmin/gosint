// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"slices"
	"testing"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

// The share dialog's search suggests the people the caller already
// works with and finds anyone else only by their whole address, so no
// signed-in account can list the directory by prefixes.
func TestUserSearchSuggestsColleaguesOnly(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv := newAPI(t, db)
		alice := signUp(t, srv, "alice@example.com")
		signUp(t, srv, "ally@example.com")
		signUp(t, srv, "a_stranger@example.com")

		var inv struct {
			Investigation idBody `json:"investigation"`
		}

		alice.expect(http.StatusCreated, "POST", "/api/investigations", map[string]string{"name": "Case"}, &inv)
		alice.expect(http.StatusCreated, "POST", "/api/investigations/"+inv.Investigation.ID+"/members",
			map[string]string{"email": "ally@example.com", "role": "viewer"})

		search := func(q string) []string {
			var out struct {
				Users []struct {
					Email string `json:"email"`
				} `json:"users"`
			}

			alice.expect(http.StatusOK, "GET", "/api/users/search?q="+q, nil, &out)

			emails := []string{}
			for _, u := range out.Users {
				emails = append(emails, u.Email)
			}

			return emails
		}

		if got := search("a"); !slices.Equal(got, []string{"ally@example.com"}) {
			t.Errorf("prefix a = %v, want the colleague only", got)
		}

		if got := search("a_"); len(got) != 0 {
			t.Errorf("prefix a_ = %v, want nothing (the underscore is literal, the stranger no colleague)", got)
		}

		if got := search("a_stranger@example.com"); !slices.Equal(got, []string{"a_stranger@example.com"}) {
			t.Errorf("full address = %v, want the stranger", got)
		}

		if got := search("alice@example.com"); len(got) != 0 {
			t.Errorf("own address = %v, want nothing", got)
		}
	})
}
