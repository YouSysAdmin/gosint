// Gosint, Copyright (c) 2026 YouSysAdmin

package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/database/dbtest"
)

const geoTemplate = `
name: geo-lookup
category: Ip
version: 1
input: {type: ip, key: address}
request:
  url: "BASE/geo/{{address}}"
response:
  map: {city: city, country: country}
output: {type: ip}
`

const keyedTemplate = `
name: keyed-lookup
category: Ip
version: 1
input: {type: ip, key: address}
secrets: [{name: GEO_KEY}]
request:
  url: "BASE/geo/{{address}}"
  headers: {X-Key: "{{secrets.GEO_KEY}}"}
response:
  map: {city: city}
output: {type: ip}
`

func geoAPI(t *testing.T) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"city":"Paris","country":"FR"}`))
	}))
	t.Cleanup(srv.Close)

	return srv.URL
}

type tplBody struct {
	Template struct {
		ID string `json:"id"`
	} `json:"template"`
}

func TestTemplateTransforms(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db database.Database) {
		srv, rt := newAPIRuntime(t, db)
		startWorker(t, rt)
		base := geoAPI(t)

		alice := signUp(t, srv, "alice@example.com")
		bob := signUp(t, srv, "bob@example.com")

		alice.expect(http.StatusBadRequest, "POST", "/api/transform-templates", map[string]any{"source": "name: [oops"})
		alice.expect(http.StatusConflict, "POST", "/api/transform-templates", map[string]any{
			"source": strings.ReplaceAll(strings.Replace(geoTemplate, "geo-lookup", "domain_to_ip", 1), "BASE", base)})

		var geo, keyed tplBody
		alice.expect(http.StatusCreated, "POST", "/api/transform-templates",
			map[string]any{"source": strings.ReplaceAll(geoTemplate, "BASE", base), "is_public": true}, &geo)
		alice.expect(http.StatusCreated, "POST", "/api/transform-templates",
			map[string]any{"source": strings.ReplaceAll(keyedTemplate, "BASE", base), "is_public": true}, &keyed)

		var tested struct {
			Outputs []struct {
				Label      string         `json:"label"`
				Properties map[string]any `json:"properties"`
				Error      string         `json:"error"`
			} `json:"outputs"`
		}

		alice.expect(http.StatusOK, "POST", "/api/transform-templates/"+geo.Template.ID+"/test", map[string]any{"value": "8.8.8.8"}, &tested)
		if len(tested.Outputs) != 1 || tested.Outputs[0].Properties["city"] != "Paris" || tested.Outputs[0].Error != "" {
			t.Errorf("test run = %+v", tested.Outputs)
		}

		var catalog struct {
			Transforms []struct {
				Name       string `json:"name"`
				TemplateID string `json:"template_id"`
			} `json:"transforms"`
		}

		bob.expect(http.StatusOK, "GET", "/api/transforms?type=ip", nil, &catalog)
		found := 0
		for _, e := range catalog.Transforms {
			if e.TemplateID != "" {
				found++
			}
		}

		if found != 2 {
			t.Errorf("bob's catalog shows %d public templates, want 2", found)
		}

		bob.expect(http.StatusForbidden, "PUT", "/api/transform-templates/"+geo.Template.ID,
			map[string]any{"source": strings.ReplaceAll(geoTemplate, "BASE", base)})

		sketch, _ := sketchFor(t, bob)

		var ip struct {
			Node idBody `json:"node"`
		}

		bob.expect(http.StatusCreated, "POST", sketch+"/nodes", map[string]any{"type": "ip", "label": "8.8.8.8"}, &ip)

		// Alice's template would read bob's GEO_KEY and send it where she chose.
		bob.expect(http.StatusForbidden, "POST", sketch+"/templates/"+keyed.Template.ID+"/launch",
			map[string]any{"node_ids": []string{ip.Node.ID}})

		var launched struct {
			Scan struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"scan"`
		}

		bob.expect(http.StatusCreated, "POST", sketch+"/templates/"+geo.Template.ID+"/launch",
			map[string]any{"node_ids": []string{ip.Node.ID}}, &launched)

		awaitScan(t, bob, sketch+"/scans/"+launched.Scan.ID, &launched)

		var g graphBody
		bob.expect(http.StatusOK, "GET", sketch+"/graph", nil, &g)
		if len(g.Nodes) != 1 || g.Nodes[0].Properties["city"] != "Paris" {
			t.Errorf("graph after the template = %+v", g.Nodes)
		}

		alice.expect(http.StatusNoContent, "DELETE", "/api/transform-templates/"+geo.Template.ID, nil)
		bob.expect(http.StatusNotFound, "GET", "/api/transform-templates/"+geo.Template.ID, nil)
	})
}

// A template moves as its YAML, several as one file with --- between
// them, and imports as the caller's own private templates.
func TestTemplatesMoveAsYAML(t *testing.T) {
	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")
	bob := signUp(t, srv, "bob@example.com")

	src := "name: github-user\ncategory: Username\ninput:\n  type: username\n  key: value\nrequest:\n  url: \"https://api.example/users/{{value}}\"\nresponse:\n  map:\n    username: login\noutput:\n  type: socialaccount\n"

	var created struct {
		Template idBody `json:"template"`
	}

	alice.expect(http.StatusCreated, "POST", "/api/transform-templates", map[string]any{"source": src}, &created)

	if code := alice.do("GET", "/api/transform-templates/"+created.Template.ID+"/export", nil, nil); code != http.StatusOK {
		t.Fatalf("export = %d", code)
	}

	file := src + "---\n" + strings.Replace(src, "github-user", "gitlab-user", 1) + "---\nname: broken\n"
	body, ct := multipartFile(t, "transforms.yaml", []byte(file))

	var result struct {
		Imported int      `json:"imported"`
		Skipped  []string `json:"skipped"`
	}

	if code := bob.do("POST", "/api/transform-templates/import", body, &result, "Content-Type", ct); code != http.StatusOK {
		t.Fatalf("import = %d", code)
	}

	if result.Imported != 2 || len(result.Skipped) != 1 {
		t.Errorf("import = %+v", result)
	}

	var list struct {
		Templates []struct {
			Name string `json:"name"`
		} `json:"templates"`
	}

	bob.expect(http.StatusOK, "GET", "/api/transform-templates", nil, &list)
	if len(list.Templates) != 2 {
		t.Errorf("bob's templates = %+v", list.Templates)
	}
}

// The editor's test sends one request whatever the template's retries
// say, so a failing API answers at once instead of after the backoff.
func TestATemplateTestRunsOnce(t *testing.T) {
	var calls atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(api.Close)

	srv := newAPI(t, dbtest.SQLite(t))
	alice := signUp(t, srv, "alice@example.com")

	var flaky tplBody
	alice.expect(http.StatusCreated, "POST", "/api/transform-templates", map[string]any{
		"source": strings.ReplaceAll(geoTemplate, "BASE", api.URL) + "retry: {max_retries: 5, backoff_factor: 2}\n"}, &flaky)

	var tested struct {
		Error  string `json:"error"`
		Answer struct {
			Status int `json:"status_code"`
		} `json:"answer"`
	}

	start := time.Now()
	alice.expect(http.StatusOK, "POST", "/api/transform-templates/"+flaky.Template.ID+"/test", map[string]any{"value": "8.8.8.8"}, &tested)
	if calls.Load() != 1 || tested.Answer.Status != http.StatusServiceUnavailable || tested.Error == "" || time.Since(start) > time.Second {
		t.Errorf("test = %+v after %d calls in %v", tested, calls.Load(), time.Since(start))
	}
}
