// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/database/dbtest"
	"github.com/yousysadmin/gosint/internal/domain/auth"
)

// fakeModel answers like an OpenAI style server, echoing how many
// messages it was shown and whether the selected entity reached it.
func fakeModel(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()

	var seen []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var req struct {
			Messages []map[string]any `json:"messages"`
		}

		_ = json.Unmarshal(body, &req)
		seen = req.Messages
		w.Header().Set("Content-Type", "text/event-stream")
		for _, piece := range []string{"The ", "answer"} {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"`+piece+`"}}]}`+"\n\n")
		}

		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	return srv, &seen
}

// A question streams back token by token, the selected entity is shown
// to the model, both turns are stored, and the first question names
// the chat. A model on this machine needs no key.
func TestChatStreamsAndStoresTheTurns(t *testing.T) {
	model, seen := fakeModel(t)
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, model.URL+"/v1", "auto")
	base, _ := sketchFor(t, alice)

	var n struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "domain", "label": "acme.io"}, &n)

	var created struct {
		Chat idBody `json:"chat"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/chats", map[string]any{}, &created)
	chat := base + "/chats/" + created.Chat.ID

	code, body, hdr := alice.raw("POST", chat+"/messages", map[string]any{
		"prompt": "What do we know about this domain?", "node_ids": []string{n.Node.ID},
	})
	if code != http.StatusOK || !strings.Contains(hdr.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream = %d %s %q", code, hdr.Get("Content-Type"), body)
	}

	if !strings.Contains(body, `event: token`) || !strings.Contains(body, `"text":"The "`) || !strings.Contains(body, "event: done") {
		t.Errorf("stream body = %q", body)
	}

	// The prompt and the entities travel as ONE system message first,
	// which a Qwen chat template insists on, then the turns.
	shown := *seen
	if len(shown) != 2 || shown[0]["role"] != "system" || !strings.Contains(shown[0]["content"].(string), "domain: acme.io") ||
		shown[1]["role"] != "user" {
		t.Errorf("the model was shown %v", shown)
	}

	var turns struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	alice.expect(http.StatusOK, "GET", chat+"/messages", nil, &turns)
	if len(turns.Messages) != 2 || turns.Messages[1].Role != "assistant" || turns.Messages[1].Content != "The answer" {
		t.Errorf("stored turns = %+v", turns.Messages)
	}

	var list struct {
		Chats []struct {
			Title    string   `json:"title"`
			Entities []string `json:"entities"`
		} `json:"chats"`
	}

	// The list says what each chat was about, so it is found again by
	// the entity and not by its first line.
	alice.expect(http.StatusOK, "GET", base+"/chats", nil, &list)
	if len(list.Chats) != 1 || list.Chats[0].Title != "What do we know about this domain?" ||
		len(list.Chats[0].Entities) != 1 || list.Chats[0].Entities[0] != n.Node.ID {
		t.Errorf("chats = %+v", list.Chats)
	}

	alice.expect(http.StatusNoContent, "DELETE", chat, nil)
	alice.expect(http.StatusNotFound, "GET", chat+"/messages", nil)
}

// addModel offers a model through the admin panel, admin being the
// first account.
func addModel(t *testing.T, admin *api, baseURL, tools string) {
	t.Helper()

	admin.expect(http.StatusCreated, "POST", "/api/admin/models", map[string]any{"name": "Test", "base_url": baseURL,
		"model": "m", "tools": tools})
}

// A remote model without a stored key is refused before anything is
// sent, naming the secret to store.
func TestChatWithoutAKeyNamesTheVaultKey(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, "https://api.example.com/v1", "auto")
	base, _ := sketchFor(t, alice)

	var created struct {
		Chat idBody `json:"chat"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/chats", map[string]any{}, &created)

	code, body, _ := alice.raw("POST", base+"/chats/"+created.Chat.ID+"/messages", map[string]any{"prompt": "hi"})
	if code != http.StatusBadRequest || !strings.Contains(body, "LLM_API_KEY") {
		t.Errorf("without a key = %d %s", code, body)
	}
}

// The generator hands back the model's YAML, tidied, and names what the
// parser still refuses instead of failing.
func TestTemplateGenerationReturnsTheYAML(t *testing.T) {
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		yaml := "```yaml\nname: my-lookup\ncategory: Ip\nversion: 1\ninput:\n  type: ip\n  key: address\nrequest:\n  method: GET\n  url: https://api.example.com/{{address}}\nresponse:\n  expect: json\n  map:\n    city: city\noutput:\n  type: ip\n```"
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":`+mustJSON(yaml)+`}}]}`+"\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(model.Close)

	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, model.URL, "auto")

	var out struct {
		Source  string `json:"source"`
		Problem string `json:"problem"`
	}

	alice.expect(http.StatusOK, "POST", "/api/transform-templates/generate",
		map[string]any{"prompt": "geolocate an ip with ip-api", "input_type": "ip", "output_type": "ip"}, &out)
	if strings.Contains(out.Source, "```") || !strings.Contains(out.Source, `url: "https://api.example.com/{{address}}"`) {
		t.Errorf("source not tidied: %q", out.Source)
	}

	if out.Problem != "" {
		t.Errorf("a valid template was refused: %s", out.Problem)
	}
}

func mustJSON(s string) string {
	b, _ := json.Marshal(s)

	return string(b)
}

// raw sends a request and hands back the answer as it came, for a body
// that is not JSON.
func (a *api) raw(method, path string, body any) (int, string, http.Header) {
	a.t.Helper()

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}

		r = strings.NewReader(string(b))
	}

	req := httptest.NewRequest(method, path, r)
	req.Header.Set("Content-Type", "application/json")
	if a.cookie != "" {
		req.Header.Set("Cookie", auth.CookieName+"="+a.cookie)
	}

	resp, err := a.srv.App().Test(req, testConfig)
	if err != nil {
		a.t.Fatalf("%s %s: %v", method, path, err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	out, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(out), resp.Header
}

// The model proposes through the tool, the proposal is stored inside
// the answer as the block, and applying it writes through the types.
func TestChatProposalsAreAppliedOnRequest(t *testing.T) {
	var shownID string
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var req struct {
			Messages []map[string]any `json:"messages"`
			Tools    []any            `json:"tools"`
		}

		_ = json.Unmarshal(body, &req)
		if len(req.Tools) != 3 {
			t.Errorf("the model was offered %d tools, want lookup, the proposal and run", len(req.Tools))
		}

		// The id the entity was shown with, in brackets.
		system, _ := req.Messages[0]["content"].(string)
		if i := strings.Index(system, "- ["); i >= 0 {
			shownID = system[i+3 : i+3+36]
		}

		args := `{"changes":[{"kind":"set_properties","node_id":"` + shownID + `","properties":{"description":"Acme's site"},"reason":"the title says so"},` +
			`{"kind":"add_entity","type":"organization","properties":{"name":"Acme Ltd"},"link_to":"` + shownID + `","relation":"OWNED_BY"}]}`
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"Acme owns it."}}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"propose_changes","arguments":`+strconv.Quote(args)+`}}]}}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(model.Close)

	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, model.URL+"/v1", "on")
	base, _ := sketchFor(t, alice)

	var n struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "domain", "label": "acme.io"}, &n)

	var created struct {
		Chat idBody `json:"chat"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/chats", map[string]any{}, &created)
	chat := base + "/chats/" + created.Chat.ID
	code, body, _ := alice.raw("POST", chat+"/messages", map[string]any{"prompt": "Who owns it?", "node_ids": []string{n.Node.ID}})
	if code != http.StatusOK || !strings.Contains(body, "event: done") || !strings.Contains(body, "gosint-changes") {
		t.Fatalf("stream = %d %q", code, body)
	}

	if shownID != n.Node.ID {
		t.Fatalf("the model was shown id %q, want %s", shownID, n.Node.ID)
	}

	// Nothing is written by the answer itself.
	var graph struct {
		Nodes []struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
		} `json:"nodes"`
		Edges []idBody `json:"edges"`
	}

	alice.expect(http.StatusOK, "GET", base+"/graph", nil, &graph)
	if len(graph.Nodes) != 1 || graph.Nodes[0].Properties["description"] != nil {
		t.Fatalf("the proposal was applied on its own: %+v", graph.Nodes)
	}

	var result struct {
		Applied int      `json:"applied"`
		Errors  []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", chat+"/apply", map[string]any{"changes": []map[string]any{
		{"kind": "set_properties", "node_id": n.Node.ID, "properties": map[string]any{"description": "Acme's site"}},
		{"kind": "add_entity", "type": "organization", "properties": map[string]any{"name": "Acme Ltd"}, "link_to": n.Node.ID, "relation": "OWNED_BY"},
		{"kind": "add_entity", "type": "nosuch", "properties": map[string]any{"name": "x"}},
	}}, &result)
	if result.Applied != 2 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "nosuch") {
		t.Errorf("apply = %+v", result)
	}

	alice.expect(http.StatusOK, "GET", base+"/graph", nil, &graph)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 {
		t.Errorf("after apply: %d nodes %d edges", len(graph.Nodes), len(graph.Edges))
	}

	for _, g := range graph.Nodes {
		if g.Type == "domain" && g.Properties["description"] != "Acme's site" {
			t.Errorf("domain after apply = %v", g.Properties)
		}
	}

	// A proposal changing the main field renames the entity like a hand
	// edit: normalized, with the key and the label following it.
	alice.expect(http.StatusOK, "POST", chat+"/apply", map[string]any{"changes": []map[string]any{
		{"kind": "set_properties", "node_id": n.Node.ID, "properties": map[string]any{"domain": "Acme.COM"}},
	}}, &result)
	if result.Applied != 1 {
		t.Fatalf("rename = %+v", result)
	}

	type keyed struct {
		ID         string         `json:"id"`
		Key        string         `json:"key"`
		Label      string         `json:"label"`
		Properties map[string]any `json:"properties"`
	}

	var after struct {
		Nodes []keyed `json:"nodes"`
	}

	alice.expect(http.StatusOK, "GET", base+"/graph", nil, &after)
	i := slices.IndexFunc(after.Nodes, func(g keyed) bool { return g.ID == n.Node.ID })
	if i < 0 || after.Nodes[i].Key != "acme.com" || after.Nodes[i].Label != "acme.com" || after.Nodes[i].Properties["domain"] != "acme.com" {
		t.Errorf("after the rename = %+v", after.Nodes)
	}
}

// A model that asks to read a page gets its text as the next turn and
// answers from it: the whole loop runs inside one streamed answer.
func TestChatLooksThingsUpBeforeAnswering(t *testing.T) {
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Jane Roe</title></head><body><p>Developer in Berlin.</p><script>x()</script></body></html>`)
	}))
	t.Cleanup(page.Close)

	var turns [][]map[string]any
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var req struct {
			Messages []map[string]any `json:"messages"`
		}

		_ = json.Unmarshal(body, &req)
		turns = append(turns, req.Messages)
		w.Header().Set("Content-Type", "text/event-stream")
		answer := "Let me read the profile.\n```gosint-actions\n{\"actions\":[{\"kind\":\"fetch_url\",\"url\":\"" + page.URL + "/jane\"}]}\n```"
		if len(turns) > 1 {
			// The second step sees the page text in the last turn.
			last, _ := req.Messages[len(req.Messages)-1]["content"].(string)
			answer = "The page says: " + firstMatch(last, "Developer in Berlin")
		}

		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":`+strconv.Quote(answer)+`}}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(model.Close)

	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, model.URL+"/v1", "off")
	base, _ := sketchFor(t, alice)

	var created struct {
		Chat idBody `json:"chat"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/chats", map[string]any{}, &created)
	chat := base + "/chats/" + created.Chat.ID
	code, body, _ := alice.raw("POST", chat+"/messages", map[string]any{"prompt": "Who is behind it?"})
	if code != http.StatusOK || !strings.Contains(body, "event: action") || !strings.Contains(body, "Reading "+page.URL) {
		t.Fatalf("stream = %d %q", code, body)
	}

	if len(turns) != 2 {
		t.Fatalf("the model was asked %d times, want 2", len(turns))
	}

	var stored struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	alice.expect(http.StatusOK, "GET", chat+"/messages", nil, &stored)
	last := stored.Messages[len(stored.Messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Let me read the profile.") ||
		!strings.Contains(last.Content, "The page says: Developer in Berlin") || strings.Contains(last.Content, "gosint-actions") {
		t.Errorf("stored answer = %q", last.Content)
	}
}

// firstMatch is needle when it is in s, else what s holds.
func firstMatch(s, needle string) string {
	if strings.Contains(s, needle) {
		return needle
	}

	return "nothing of " + s
}

// scriptedModel answers in block mode: the first round's actions are
// picked by the question, the second round repeats what came back, so a
// test reads the run's result off the stored answer.
func scriptedModel(t *testing.T, scripts map[string]string) (*httptest.Server, *[]string) {
	t.Helper()

	var systems []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var req struct {
			Messages []map[string]any `json:"messages"`
		}

		_ = json.Unmarshal(body, &req)
		system, _ := req.Messages[0]["content"].(string)
		systems = append(systems, system)
		last, _ := req.Messages[len(req.Messages)-1]["content"].(string)
		answer := "Done. " + last
		if block, ok := scripts[last]; ok {
			answer = "On it.\n```gosint-actions\n" + block + "\n```"
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":`+strconv.Quote(answer)+`}}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	return srv, &systems
}

// The model runs a transform or a flow it composed, the server waits
// for the scan and hands back what was written, a chain that cannot run
// is refused with the reason, a run that contacts the target becomes a
// card, and a viewer's chat runs nothing.
func TestChatRunsTransformsForTheModel(t *testing.T) {
	model, systems := scriptedModel(t, map[string]string{
		"run email_to_domain": `{"actions":[{"kind":"run_transform","transform":"email_to_domain"}]}`,
		"run flow": `{"actions":[{"kind":"run_flow","steps":[{"id":"dom","transform":"email_to_domain","after":"input"},` +
			`{"id":"keep","transform":"domain_root_only","after":"dom"}]}]}`,
		"run bad flow": `{"actions":[{"kind":"run_flow","input":"email","steps":[{"id":"dom","transform":"email_to_domain","after":"input"},` +
			`{"id":"asn","transform":"ip_to_asn","after":"dom"}]}]}`,
		"run ports": `{"actions":[{"kind":"run_transform","transform":"ip_to_ports","params":{"source":"active"}}]}`,
		"run n8n":   `{"actions":[{"kind":"run_transform","transform":"n8n_connector","params":{"webhook_url":"https://hooks.example.com/x"}}]}`,
	})
	srv, rt := newAPIRuntime(t, dbtest.SQLite(t))
	startWorker(t, rt)
	alice := signUp(t, srv, "alice@example.com")
	addModel(t, alice, model.URL+"/v1", "off")
	bob := signUp(t, srv, "bob@example.com")
	base, _ := sketchFor(t, alice)

	var email, ip struct {
		Node idBody `json:"node"`
	}

	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "email", "label": "jane@example.com"}, &email)
	alice.expect(http.StatusCreated, "POST", base+"/nodes", map[string]any{"type": "ip", "label": "198.51.100.7"}, &ip)

	ask := func(a *api, prompt string, nodeIDs ...string) string {
		t.Helper()

		var created struct {
			Chat idBody `json:"chat"`
		}

		a.expect(http.StatusCreated, "POST", base+"/chats", map[string]any{}, &created)
		code, body, _ := a.raw("POST", base+"/chats/"+created.Chat.ID+"/messages", map[string]any{"prompt": prompt, "node_ids": nodeIDs})
		if code != http.StatusOK || !strings.Contains(body, "event: done") {
			t.Fatalf("%s: stream = %d %q", prompt, code, body)
		}

		var stored struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}

		a.expect(http.StatusOK, "GET", base+"/chats/"+created.Chat.ID+"/messages", nil, &stored)

		return body + "\n--- stored ---\n" + stored.Messages[len(stored.Messages)-1].Content
	}

	out := ask(alice, "run email_to_domain", email.Node.ID)
	for _, want := range []string{"event: action", "Running email_to_domain on 1 entity", "email_to_domain completed", "domain: example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("run: missing %q in %s", want, out)
		}
	}

	if !strings.Contains((*systems)[0], "Transforms you may run") || !strings.Contains((*systems)[0], "email_to_domain: email -> domain") {
		t.Errorf("the catalog was not shown: %s", (*systems)[0])
	}

	// The flow's input is taken from its first step, each step reports,
	// and the filter passed the root domain on.
	out = ask(alice, "run flow", email.Node.ID)
	for _, want := range []string{"Running a 2-step flow on 1 entity", "inline: email_to_domain, domain_root_only completed", "step keep (domain_root_only, a filter): 1 in, 1 kept and passed on, 0 dropped"} {
		if !strings.Contains(out, want) {
			t.Errorf("flow: missing %q in %s", want, out)
		}
	}

	out = ask(alice, "run bad flow", email.Node.ID)
	if !strings.Contains(out, "ip_to_asn, which takes ip, after a step producing domain") {
		t.Errorf("bad flow: %s", out)
	}

	// An active scan is the person's to start: a card, no scan.
	out = ask(alice, "run ports", ip.Node.ID)
	if !strings.Contains(out, "gosint-changes") || !strings.Contains(out, `"kind":"run"`) || !strings.Contains(out, `"source":"active"`) {
		t.Errorf("active run: %s", out)
	}

	// So is a run posting the entities to a URL the model chose.
	out = ask(alice, "run n8n", ip.Node.ID)
	if !strings.Contains(out, "gosint-changes") || !strings.Contains(out, `"kind":"run"`) || !strings.Contains(out, `"transform":"n8n_connector"`) {
		t.Errorf("n8n run: %s", out)
	}

	var scans struct {
		Scans []struct {
			Name string `json:"name"`
		} `json:"scans"`
	}

	alice.expect(http.StatusOK, "GET", base+"/scans", nil, &scans)
	if len(scans.Scans) != 2 || slices.ContainsFunc(scans.Scans, func(s struct {
		Name string `json:"name"`
	}) bool {
		return s.Name == "ip_to_ports" || s.Name == "n8n_connector"
	}) {
		t.Errorf("scans after the chats = %+v", scans.Scans)
	}

	// Confirming a run card queues it as a launch would.
	var chats struct {
		Chats []idBody `json:"chats"`
	}

	alice.expect(http.StatusOK, "GET", base+"/chats", nil, &chats)

	var applied struct {
		Applied  int      `json:"applied"`
		Launched int      `json:"launched"`
		Errors   []string `json:"errors"`
	}

	alice.expect(http.StatusOK, "POST", base+"/chats/"+chats.Chats[0].ID+"/apply", map[string]any{"changes": []map[string]any{
		{"kind": "run", "transform": "email_to_domain", "node_ids": []string{email.Node.ID}},
	}}, &applied)
	if applied.Launched != 1 || len(applied.Errors) != 0 {
		t.Errorf("apply run = %+v", applied)
	}

	alice.expect(http.StatusOK, "GET", base+"/scans", nil, &scans)
	if len(scans.Scans) != 3 {
		t.Errorf("scans after the card = %+v", scans.Scans)
	}

	// A viewer's model is told the role is read only and a run it asks
	// for anyway is refused.
	alice.expect(http.StatusCreated, "POST", investigationOf(t, alice, base)+"/members", map[string]string{"email": "bob@example.com", "role": "viewer"})

	out = ask(bob, "run email_to_domain", email.Node.ID)
	if !strings.Contains(out, "read only") || strings.Contains(out, "event: action") {
		t.Errorf("viewer: %s", out)
	}

	if last := (*systems)[len(*systems)-1]; !strings.Contains(last, "read only") || strings.Contains(last, "Transforms you may run") {
		t.Errorf("the viewer's model was told: %s", last)
	}
}

// investigationOf is the API path of the sketch's investigation.
func investigationOf(t *testing.T, a *api, sketch string) string {
	t.Helper()

	var got struct {
		Sketch struct {
			InvestigationID string `json:"investigation_id"`
		} `json:"sketch"`
	}

	a.expect(http.StatusOK, "GET", sketch, nil, &got)

	return "/api/investigations/" + got.Sketch.InvestigationID
}
