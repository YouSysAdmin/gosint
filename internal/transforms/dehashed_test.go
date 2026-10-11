// Gosint, Copyright (c) 2026 YouSysAdmin

package transforms_test

import (
	"encoding/json/v2"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	"github.com/yousysadmin/gosint/internal/transforms/email"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/phone"
)

// A record draws its breach, the login it leaked with the password and
// hash as a credential under the breach, and each leaked fact once. The
// input is never linked to itself.
func TestDehashed(t *testing.T) {
	h := newHarness(t)
	base := api(t, map[string]func(http.ResponseWriter, *http.Request){
		"/v2/search": func(w http.ResponseWriter, r *http.Request) {
			var q struct {
				Query string `json:"query"`
				Size  int    `json:"size"`
			}

			err := json.UnmarshalRead(r.Body, &q)
			ok := q.Query == `email:"leaked@example.com"` || q.Query == `phone:"33612345678"`
			if r.Method != http.MethodPost || r.Header.Get("Dehashed-Api-Key") != "secret" || err != nil || !ok || q.Size != 50 {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			_, _ = w.Write([]byte(`{"balance": 41, "total": 2, "entries": [
				{"email": ["leaked@example.com", "Other@Example.com"], "name": ["Jane Roe"], "dob": ["1990-01-02"],
				 "username": ["jroe"], "phone": ["+33 6 12 34 56 78"], "ip_address": ["203.0.113.7"],
				 "password": ["hunter2"], "hashed_password": ["5f4dcc3b5aa765d61d8327deb882cf99"], "database_name": "ExampleLeak"},
				{"email": ["leaked@example.com"], "username": ["jroe"], "database_name": "OtherLeak"}]}`))
		},
	})

	client := dehashed.Client{BaseURL: base + "v2/search"}
	h.run(email.EmailToDehashed{Client: client}, map[string]string{"DEHASHED_API_KEY": "secret", "max_results": "50"},
		h.node("email", map[string]any{"email": "leaked@example.com"}))

	h.expectEdges(
		"leaked@example.com -FOUND_IN_BREACH-> ExampleLeak",
		"leaked@example.com -FOUND_IN_BREACH-> OtherLeak",
		"ExampleLeak -HAS_CREDENTIAL-> leaked@example.com@ExampleLeak",
		"leaked@example.com -LEAKED_WITH-> Jane Roe",
		"leaked@example.com -LEAKED_WITH-> other@example.com",
		"leaked@example.com -LEAKED_WITH-> jroe",
		"leaked@example.com -LEAKED_WITH-> +33612345678",
		"leaked@example.com -LEAKED_WITH-> 203.0.113.7",
	)

	nodes, _, err := h.st.Graph.Graph(t.Context(), h.sketch)
	if err != nil {
		t.Fatal(err)
	}

	var cred map[string]any
	for _, n := range nodes {
		if n.Type == "credential" {
			cred = n.Properties
		}
	}

	if cred["password"] != "hunter2" || cred["password_hash"] != "5f4dcc3b5aa765d61d8327deb882cf99" || cred["found_in"] != "ExampleLeak" {
		t.Errorf("credential = %v", cred)
	}

	// A refused key fails the scan instead of warning once per email.
	x := transform.NewExec(transform.ExecConfig{SketchID: h.sketch, Graph: h.st.Graph, Types: types.Builtin(), Log: &h.log,
		Params: map[string]string{"DEHASHED_API_KEY": "wrong"}, HTTP: transform.DefaultHTTPClient("gosint-test")})
	err = email.EmailToDehashed{Client: client}.Run(t.Context(), x,
		[]graphmodel.Node{h.node("email", map[string]any{"email": "leaked@example.com"})})
	if err == nil || !strings.Contains(err.Error(), "refused the key") {
		t.Errorf("a wrong key = %v", err)
	}

	// A phone is searched as digits, and the phone the record holds is the
	// input itself, not linked to itself.
	p := newHarness(t)
	p.run(phone.PhoneToDehashed{Client: client}, map[string]string{"DEHASHED_API_KEY": "secret", "max_results": "50"},
		p.node("phone", map[string]any{"number": "+33 6 12 34 56 78"}))
	got := p.edges()
	for _, e := range got {
		if strings.HasPrefix(e, "+33612345678 -LEAKED_WITH-> +33612345678") {
			t.Errorf("the input linked to itself: %q", got)
		}
	}

	if !slices.Contains(got, "+33612345678 -LEAKED_WITH-> leaked@example.com") ||
		!slices.Contains(got, "ExampleLeak -HAS_CREDENTIAL-> leaked@example.com@ExampleLeak") {
		t.Errorf("phone edges = %q", got)
	}
}
