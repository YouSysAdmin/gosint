// Gosint, Copyright (c) 2026 YouSysAdmin

// Package nucleireport reads the results nuclei writes, one JSON object
// per line (-jsonl) or a JSON array of them (-json-export). A line that
// does not read is counted, not fatal: a report cut short still brings
// in what it holds.
package nucleireport

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Result is one match: a template that fired on a target.
type Result struct {
	TemplateID string
	Name       string
	Severity   string
	Type       string // http, dns, network, ssl, ...

	Description string
	Tags        []string
	References  []string
	CVEs        []string
	CWEs        []string
	CVSS        float64

	Host      string // the target as given, https://example.com or example.com:22
	MatchedAt string // where the match is, a URL or host:port
	IP        string
	Port      string
	Matcher   string
	Extracted []string
	Timestamp time.Time
}

// line is nuclei's JSON. Several fields come as a string in one version
// and a list in another, or null, so those are read loosely.
type line struct {
	TemplateID string `json:"template-id"`
	Info       struct {
		Name           string `json:"name"`
		Severity       string `json:"severity"`
		Description    string `json:"description"`
		Tags           any    `json:"tags"`
		Reference      any    `json:"reference"`
		Classification struct {
			CVE       any     `json:"cve-id"`
			CWE       any     `json:"cwe-id"`
			CVSSScore float64 `json:"cvss-score"`
		} `json:"classification"`
	} `json:"info"`
	Type      string `json:"type"`
	Host      string `json:"host"`
	MatchedAt string `json:"matched-at"`
	IP        string `json:"ip"`
	Port      any    `json:"port"`
	Matcher   string `json:"matcher-name"`
	Extracted any    `json:"extracted-results"`
	Timestamp string `json:"timestamp"`
}

// Report is the results of one file and how many lines were unreadable.
type Report struct {
	Results []Result
	Skipped int
}

// Parse reads a -jsonl or -json-export file.
func Parse(body []byte) (*Report, error) {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return nil, errors.New("the report is empty")
	}

	var lines []line
	r := &Report{}
	if body[0] == '[' {
		if err := json.Unmarshal(body, &lines); err != nil {
			return nil, fmt.Errorf("not a nuclei JSON export: %w", err)
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
		for sc.Scan() {
			raw := bytes.TrimSpace(sc.Bytes())
			if len(raw) == 0 {
				continue
			}

			var l line
			if err := json.Unmarshal(raw, &l); err != nil {
				r.Skipped++

				continue
			}

			lines = append(lines, l)
		}

		if err := sc.Err(); err != nil {
			return nil, err
		}
	}

	for _, l := range lines {
		if l.TemplateID == "" && l.Info.Name == "" {
			r.Skipped++

			continue
		}

		res := Result{
			TemplateID:  l.TemplateID,
			Name:        strings.TrimSpace(l.Info.Name),
			Severity:    strings.ToLower(strings.TrimSpace(l.Info.Severity)),
			Type:        l.Type,
			Description: strings.TrimSpace(l.Info.Description),
			Tags:        list(l.Info.Tags),
			References:  list(l.Info.Reference),
			CVEs:        upper(list(l.Info.Classification.CVE)),
			CWEs:        upper(list(l.Info.Classification.CWE)),
			CVSS:        l.Info.Classification.CVSSScore,
			Host:        l.Host,
			MatchedAt:   l.MatchedAt,
			IP:          l.IP,
			Port:        scalar(l.Port),
			Matcher:     l.Matcher,
			Extracted:   list(l.Extracted),
		}
		if res.Name == "" {
			res.Name = res.TemplateID
		}

		if t, err := time.Parse(time.RFC3339Nano, l.Timestamp); err == nil {
			res.Timestamp = t.UTC()
		}

		r.Results = append(r.Results, res)
	}

	if len(r.Results) == 0 {
		return nil, errors.New("no nuclei result in the file, it takes -jsonl or -json-export output")
	}

	return r, nil
}

// list reads a value nuclei writes as a string, a comma list, a list or
// null.
func list(v any) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}

	switch v := v.(type) {
	case string:
		for part := range strings.SplitSeq(v, ",") {
			add(part)
		}
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok {
				add(s)
			}
		}
	}

	return out
}

func upper(in []string) []string {
	for i, s := range in {
		in[i] = strings.ToUpper(s)
	}

	return in
}

// scalar reads a port written as "443" or 443.
func scalar(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	}

	return ""
}
