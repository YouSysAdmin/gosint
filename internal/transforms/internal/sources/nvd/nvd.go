// Gosint, Copyright (c) 2026 YouSysAdmin

// Package nvd is the NVD CVE API 2.0: a CVE's record read into the
// fields the cve and vulnerability types share.
package nvd

import (
	"cmp"
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the API.
const URL = "https://services.nvd.nist.gov/rest/json/cves/2.0"

// Page is where a person reads a record.
const Page = "https://nvd.nist.gov/vuln/detail/"

// KeyParam is the optional key every NVD transform takes.
var KeyParam = transform.Param{Name: "NVD_API_KEY", Type: transform.ParamSecret,
	Description: "Optional, raises the NVD's rate limit from 5 to 50 requests in 30 seconds."}

// The NVD lets 5 requests in 30 seconds through without a key, 50 with
// one, so a run spaces its requests to stay under.
const (
	paceWithoutKey = 6 * time.Second
	paceWithKey    = 700 * time.Millisecond
	maxReferences  = 20
)

// Shared are the fields a vulnerability takes over from a CVE record.
var Shared = []string{"severity", "cvss", "cvss_vector", "cwe", "description", "references"}

// Public is the client of the NVD itself.
var Public = Client{BaseURL: URL}

// Client talks to one NVD. Pace overrides the spacing between
// requests, tests set it to 0.
type Client struct {
	BaseURL string
	Pace    *time.Duration
}

// Each looks the ids up in turn, spaced for the rate limit, and hands
// each record found to fn as the fields it fills. A miss or a failure
// is logged and the next id goes on.
func (c Client) Each(ctx context.Context, x *transform.Exec, ids []string, fn func(i int, id string, fields map[string]any)) error {
	key := x.Param(KeyParam.Name)
	pace := paceWithoutKey
	if key != "" {
		pace = paceWithKey
	}

	if c.Pace != nil {
		pace = *c.Pace
	}

	var header http.Header
	if key != "" {
		header = http.Header{"apiKey": {key}}
	}

	asked := 0
	for i, id := range ids {
		if id == "" {
			continue
		}

		if asked > 0 && pace > 0 {
			timer := time.NewTimer(pace)
			select {
			case <-ctx.Done():
				timer.Stop()

				return ctx.Err()
			case <-timer.C:
			}
		}

		asked++
		var a answer
		found, err := httpx.GetJSON(ctx, x, c.BaseURL+"?"+url.Values{"cveId": {id}}.Encode(), header, &a)
		if err != nil {
			x.Warn("%s: %v", id, err)

			continue
		}

		if !found || len(a.Vulnerabilities) == 0 {
			x.Warn("%s: the NVD has no such record", id)

			continue
		}

		fn(i, id, details(a))
	}

	return nil
}

type answer struct {
	Vulnerabilities []struct {
		CVE struct {
			ID           string `json:"id"`
			Published    string `json:"published"`
			Descriptions []struct {
				Lang  string `json:"lang"`
				Value string `json:"value"`
			} `json:"descriptions"`
			Metrics struct {
				V40 []metric `json:"cvssMetricV40"`
				V31 []metric `json:"cvssMetricV31"`
				V30 []metric `json:"cvssMetricV30"`
				V2  []metric `json:"cvssMetricV2"`
			} `json:"metrics"`
			Weaknesses []struct {
				Description []struct {
					Value string `json:"value"`
				} `json:"description"`
			} `json:"weaknesses"`
			References []struct {
				URL string `json:"url"`
			} `json:"references"`
			CISAExploitAdd string `json:"cisaExploitAdd"`
		} `json:"cve"`
	} `json:"vulnerabilities"`
}

type metric struct {
	Type     string `json:"type"`
	CVSSData struct {
		BaseScore    float64 `json:"baseScore"`
		BaseSeverity string  `json:"baseSeverity"`
		Vector       string  `json:"vectorString"`
	} `json:"cvssData"`
	BaseSeverity string `json:"baseSeverity"`
}

// details reads the record into the cve type's fields.
func details(a answer) map[string]any {
	c := a.Vulnerabilities[0].CVE
	p := map[string]any{"url": Page + c.ID}
	for _, d := range c.Descriptions {
		if d.Lang == "en" {
			p["description"] = d.Value

			break
		}
	}

	// The newest CVSS version the record has, the NVD's own ("Primary")
	// score before a vendor's.
	for _, set := range [][]metric{c.Metrics.V40, c.Metrics.V31, c.Metrics.V30, c.Metrics.V2} {
		if len(set) == 0 {
			continue
		}

		m := set[0]
		for _, s := range set {
			if s.Type == "Primary" {
				m = s

				break
			}
		}

		p["cvss"] = m.CVSSData.BaseScore
		p["cvss_vector"] = m.CVSSData.Vector
		// The NVD rates a 0.0 score NONE, which the scale calls info.
		sev := strings.ToLower(cmp.Or(m.CVSSData.BaseSeverity, m.BaseSeverity))
		if sev == "none" {
			sev = "info"
		}

		if slices.Contains(types.Severities, sev) {
			p["severity"] = sev
		}

		break
	}

	for _, w := range c.Weaknesses {
		for _, d := range w.Description {
			if strings.HasPrefix(d.Value, "CWE-") {
				p["cwe"] = d.Value

				break
			}
		}

		if p["cwe"] != nil {
			break
		}
	}

	if t, err := time.Parse("2006-01-02T15:04:05.000", c.Published); err == nil {
		p["published"] = t.UTC().Format(time.RFC3339)
	}

	var refs []any
	for _, r := range c.References {
		if len(refs) < maxReferences && r.URL != "" {
			refs = append(refs, r.URL)
		}
	}

	if len(refs) > 0 {
		p["references"] = refs
	}

	p["known_exploited"] = c.CISAExploitAdd != ""
	if t, err := time.Parse("2006-01-02", c.CISAExploitAdd); err == nil {
		p["kev_added"] = t.UTC().Format(time.RFC3339)
	}

	return p
}
