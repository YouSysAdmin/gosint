// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/envelope"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// postRaw posts a JSON body and returns the status, body and headers.
func (a *api) postRaw(path string, body any) (int, []byte, http.Header) {
	a.t.Helper()

	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", auth.CookieName+"="+a.cookie)

	resp, err := a.srv.App().Test(req, testConfig)
	if err != nil {
		a.t.Fatal(err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	raw, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, raw, resp.Header
}

// importFile posts an export file with a passphrase as the multipart an
// import takes.
func (a *api) importFile(path, name string, data []byte, passphrase string, out any) int {
	a.t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", name)
	if err != nil {
		a.t.Fatal(err)
	}

	_, _ = part.Write(data)
	_ = mw.WriteField("passphrase", passphrase)
	_ = mw.Close()

	return a.do("POST", path, body.Bytes(), out, "Content-Type", mw.FormDataContentType())
}

// An investigation exports as one file, sealed under a passphrase, and
// imports as a new investigation with its sketches, graphs, files,
// pictures and analyses, the ids inside the analyses following.
func TestAnInvestigationMovesAsOneFile(t *testing.T) {
	srv, _ := newAPIRuntime(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	sketch, sketchID := sketchFor(t, alice)
	inv := investigationOf(t, alice, sketch)

	var a, b struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "domain", "label": "example.com",
		"metadata": map[string]any{"notes": "the target"}}, &a)
	alice.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "ip", "label": "93.184.216.34"}, &b)
	alice.expect(http.StatusCreated, "POST", sketch+"/edges", map[string]any{"source": a.Node.ID, "target": b.Node.ID,
		"label": "RESOLVES_TO", "properties": map[string]any{"ttl": 300}})

	var up struct {
		File struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"file"`
	}

	if code := alice.upload(sketch, a.Node.ID, "logo.png", "\x89PNG\r\n\x1a\nlogo", &up); code != http.StatusCreated {
		t.Fatalf("upload = %d", code)
	}

	alice.expect(http.StatusOK, "PATCH", sketch+"/nodes/"+a.Node.ID, map[string]any{"image": up.File.URL})

	doc := map[string]any{"type": "doc", "content": []any{map[string]any{"type": "mention",
		"attrs": map[string]any{"id": a.Node.ID, "sketch": sketchID, "label": "example.com"}}}}
	alice.expect(http.StatusCreated, "POST", inv+"/analyses", map[string]any{"title": "Findings", "content": doc})

	// Sealed: the file carries no plain text and the headers name it.
	code, sealed, h := alice.postRaw(inv+"/export", map[string]any{"passphrase": "correct horse"})
	if code != http.StatusOK || !envelope.IsSealed(sealed) || bytes.Contains(sealed, []byte("example.com")) {
		t.Fatalf("export = %d sealed=%v", code, envelope.IsSealed(sealed))
	}

	if cd := h.Get("Content-Disposition"); !strings.Contains(cd, "Case.gosint") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	if code, _, _ := bob.postRaw(inv+"/export", map[string]any{}); code != http.StatusNotFound {
		t.Errorf("a non-member exported: %d", code)
	}

	// The wrong passphrase and a missing one are refused.
	var refused struct {
		Error string `json:"error"`
	}

	if code := bob.importFile("/api/investigations/import", "Case.gosint", sealed, "wrong", &refused); code != http.StatusBadRequest ||
		!strings.Contains(refused.Error, "passphrase") {
		t.Errorf("wrong passphrase = %d %q", code, refused.Error)
	}

	if code := bob.importFile("/api/investigations/import", "Case.gosint", sealed, "", &refused); code != http.StatusBadRequest ||
		!strings.Contains(refused.Error, "encrypted") {
		t.Errorf("no passphrase = %d %q", code, refused.Error)
	}

	var made struct {
		Investigation struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"current_user_role"`
		} `json:"investigation"`
		Sketches []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sketches"`
		NodesCreated    int      `json:"nodes_created"`
		EdgesCreated    int      `json:"edges_created"`
		FilesCreated    int      `json:"files_created"`
		AnalysesCreated int      `json:"analyses_created"`
		Errors          []string `json:"errors"`
	}

	if code := bob.importFile("/api/investigations/import", "Case.gosint", sealed, "correct horse", &made); code != http.StatusCreated {
		t.Fatalf("import = %d", code)
	}

	if made.Investigation.Name != "Case" || made.Investigation.Role != "owner" || len(made.Sketches) != 1 ||
		made.Sketches[0].Title != "Map" || made.NodesCreated != 2 || made.EdgesCreated != 1 || made.FilesCreated != 1 ||
		made.AnalysesCreated != 1 || len(made.Errors) != 0 {
		t.Fatalf("import = %+v", made)
	}

	// Bob owns the copy, with the notes, the edge's properties, the
	// file and the picture pointing at the copied file.
	copied := "/api/sketches/" + made.Sketches[0].ID

	var graph struct {
		Nodes []struct {
			ID       string         `json:"id"`
			Label    string         `json:"label"`
			Image    string         `json:"image"`
			Metadata map[string]any `json:"metadata"`
		} `json:"nodes"`
		Edges []struct {
			Properties map[string]any `json:"properties"`
		} `json:"edges"`
	}

	bob.expect(http.StatusOK, "GET", copied+"/graph", nil, &graph)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].Properties["ttl"] != float64(300) {
		t.Fatalf("copied graph = %+v", graph)
	}

	var domain string
	for _, n := range graph.Nodes {
		if n.Label == "example.com" {
			domain = n.ID
			if n.Metadata["notes"] != "the target" {
				t.Errorf("notes = %v", n.Metadata)
			}

			if !strings.HasPrefix(n.Image, copied+"/files/") {
				t.Errorf("image = %q, want a file of the copy", n.Image)
			}

			if code, body, _ := bob.fetchRaw(n.Image); code != http.StatusOK || !strings.HasSuffix(string(body), "logo") {
				t.Errorf("the picture does not serve: %d", code)
			}
		}
	}

	// The analysis names the copied sketch and entity.
	var list struct {
		Analyses []struct {
			ID string `json:"id"`
		} `json:"analyses"`
	}

	bob.expect(http.StatusOK, "GET", "/api/investigations/"+made.Investigation.ID+"/analyses", nil, &list)
	if len(list.Analyses) != 1 {
		t.Fatalf("analyses = %+v", list)
	}

	var analysis struct {
		Analysis struct {
			Content map[string]any `json:"content"`
		} `json:"analysis"`
	}

	bob.expect(http.StatusOK, "GET", "/api/investigations/"+made.Investigation.ID+"/analyses/"+list.Analyses[0].ID, nil, &analysis)
	content, _ := json.Marshal(analysis.Analysis.Content)
	if !strings.Contains(string(content), domain) || !strings.Contains(string(content), made.Sketches[0].ID) ||
		strings.Contains(string(content), a.Node.ID) {
		t.Errorf("analysis content = %s, want the copied ids", content)
	}

	if code, _, _ := alice.postRaw("/api/investigations/"+made.Investigation.ID+"/export", map[string]any{}); code != http.StatusNotFound {
		t.Errorf("alice sees bob's copy: %d", code)
	}

	// A plain sketch export imports onto an investigation as a new
	// sketch named as the file says, and into a sketch as a merge.
	code, plain, _ := alice.postRaw(sketch+"/export", map[string]any{})
	if code != http.StatusOK || envelope.IsSealed(plain) {
		t.Fatalf("plain export = %d", code)
	}

	var imported struct {
		Sketch struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"sketch"`
		Result struct {
			NodesCreated int `json:"nodes_created"`
		} `json:"result"`
	}

	if code := alice.importFile(inv+"/sketches/import", "whatever.json", plain, "", &imported); code != http.StatusCreated {
		t.Fatalf("sketch import = %d", code)
	}

	if imported.Sketch.Title != "Map" || imported.Result.NodesCreated != 2 || imported.Sketch.ID == sketchID {
		t.Errorf("sketch import = %+v", imported)
	}

	var merged struct {
		NodesCreated int `json:"nodes_created"`
	}

	if code := alice.importFile(sketch+"/import/file", "Map.json", plain, "", &merged); code != http.StatusOK ||
		merged.NodesCreated != 2 {
		t.Errorf("merge import = %d %+v", code, merged)
	}

	if code := alice.importFile(inv+"/sketches/import", "Case.gosint", sealed, "correct horse", &refused); code != http.StatusBadRequest ||
		!strings.Contains(refused.Error, "investigation export") {
		t.Errorf("an investigation file as a sketch = %d %q", code, refused.Error)
	}

	if code := bob.importFile(inv+"/sketches/import", "Map.json", plain, "", &refused); code != http.StatusNotFound {
		t.Errorf("a non-member imported a sketch: %d", code)
	}
}

// A CSV imports by a mapping: each row makes the entities the mapping
// names and the relationships between them, a repeated value is one
// entity, a row without the main field skips that entity, and a list
// field splits on semicolons.
func TestATableImportsByAMapping(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	sketch, _ := sketchFor(t, alice)

	csv := "Name;Email;Domain;Aliases\nJane Roe;jane@example.com;example.com;jr|janie\nJohn Doe;john@example.com;example.com;\nNo Mail;;other.org;\n"
	form := func(mapping string) ([]byte, string) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("file", "people.csv")
		_, _ = part.Write([]byte(csv))
		if mapping != "" {
			_ = mw.WriteField("mapping", mapping)
		}

		_ = mw.Close()

		return body.Bytes(), mw.FormDataContentType()
	}

	var preview struct {
		Columns []string   `json:"columns"`
		Rows    [][]string `json:"rows"`
		Total   int        `json:"total"`
		Guesses []struct {
			Column string `json:"column"`
			Type   string `json:"type"`
		} `json:"guesses"`
	}

	// expect takes no headers, so the multipart calls go through do.
	body, ct := form("")
	if code := alice.do("POST", sketch+"/import/table/preview", body, &preview, "Content-Type", ct); code != http.StatusOK {
		t.Fatalf("preview = %d", code)
	}
	if len(preview.Columns) != 4 || preview.Total != 3 || len(preview.Rows) != 3 {
		t.Fatalf("preview = %+v", preview)
	}

	guessed := map[string]string{}
	for _, g := range preview.Guesses {
		guessed[g.Column] = g.Type
	}

	if guessed["Email"] != "email" || guessed["Domain"] != "domain" {
		t.Errorf("guesses = %v", guessed)
	}

	mapping := `{"entities":[
		{"type":"individual","fields":{"full_name":"Name","aliases":"Aliases"}},
		{"type":"email","fields":{"email":"Email"}},
		{"type":"domain","fields":{"domain":"Domain"}}],
	  "links":[{"from":0,"to":1,"label":"HAS_EMAIL"},{"from":1,"to":2,"label":"HAS_DOMAIN"}]}`

	var result struct {
		NodesCreated int      `json:"nodes_created"`
		EdgesCreated int      `json:"edges_created"`
		Errors       []string `json:"errors"`
	}

	body, ct = form(mapping)
	if code := alice.do("POST", sketch+"/import/table", body, &result, "Content-Type", ct); code != http.StatusOK {
		t.Fatalf("import = %d", code)
	}

	var g graphBody
	alice.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
	byLabel := map[string]map[string]any{}
	for _, n := range g.Nodes {
		byLabel[n.Type+":"+n.Label] = n.Properties
	}

	// Three people, two emails, two domains (example.com once), and the
	// four relationships, the third row's email skipped.
	if len(g.Nodes) != 7 || len(g.Edges) != 4 {
		t.Errorf("imported %d nodes and %d edges: %v, result %+v", len(g.Nodes), len(g.Edges), byLabel, result)
	}

	if aliases, _ := byLabel["individual:Jane Roe"]["aliases"].([]any); len(aliases) != 2 || aliases[0] != "jr" {
		t.Errorf("aliases = %v", byLabel["individual:Jane Roe"]["aliases"])
	}

	if _, ok := byLabel["domain:other.org"]; !ok {
		t.Errorf("the third row's domain is missing: %v", byLabel)
	}

	// A mapping naming a column the table lacks is refused whole.
	body, ct = form(`{"entities":[{"type":"email","fields":{"email":"Mail"}}]}`)
	if code := alice.do("POST", sketch+"/import/table", body, nil, "Content-Type", ct); code != http.StatusBadRequest {
		t.Errorf("a mapping naming a missing column = %d, want 400", code)
	}
}
