// Gosint, Copyright (c) 2026 YouSysAdmin

// Package epss is FIRST's Exploit Prediction Scoring System: the odds a
// CVE is exploited in the next thirty days, and where that sits among
// all CVEs.
package epss

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/internal/httpx"
)

// URL is the API, no key needed.
const URL = "https://api.first.org/data/v1/epss"

// Public is the client of FIRST itself.
var Public = Client{BaseURL: URL}

// Client talks to one EPSS API.
type Client struct {
	BaseURL string
}

// Score is one CVE's rating.
type Score struct {
	EPSS       float64
	Percentile float64
}

// Lookup rates the ids given, up to 100 in one request, by id. An id
// FIRST has no score for is absent.
func (c Client) Lookup(ctx context.Context, x *transform.Exec, ids []string) (map[string]Score, error) {
	out := map[string]Score{}
	for i := 0; i < len(ids); i += 100 {
		var answer struct {
			Data []struct {
				CVE        string `json:"cve"`
				EPSS       string `json:"epss"`
				Percentile string `json:"percentile"`
			} `json:"data"`
		}

		batch := ids[i:min(i+100, len(ids))]
		u := c.BaseURL + "?" + url.Values{"cve": {strings.Join(batch, ",")}}.Encode()
		if _, err := httpx.GetJSON(ctx, x, u, nil, &answer); err != nil {
			return nil, err
		}

		for _, d := range answer.Data {
			e, err1 := strconv.ParseFloat(d.EPSS, 64)
			p, err2 := strconv.ParseFloat(d.Percentile, 64)
			if err1 == nil && err2 == nil {
				out[strings.ToUpper(d.CVE)] = Score{EPSS: e, Percentile: p}
			}
		}
	}

	return out, nil
}
