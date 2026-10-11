// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"cmp"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/nucleireport"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// severityFlag colours a finding by how bad it would be, so the list's
// flag chips pick out the critical ones. info gets none.
var severityFlag = map[string]string{"critical": "red", "high": "orange", "medium": "yellow", "low": "blue"}

// ImportNuclei adds the results of a nuclei run (-jsonl or -json-export),
// sent as the request body. Each result is a FINDING, unverified, under
// what it was found on: the website for an http template, else the
// domain or the address. The CVEs a template names are CVE entities the
// finding references. A report made elsewhere lands in the sketch
// without the server scanning anything.
func (h Handler) ImportNuclei(c fiber.Ctx) error {
	body := c.Body()
	if len(body) > maxNmapReport {
		return response.BadRequest(c, "the report is over 8 MB")
	}

	report, err := nucleireport.Parse(body)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	in, unplaced := nucleiImport(report)
	in.Source = "nuclei"
	if report.Skipped > 0 {
		in.Notes = append(in.Notes, fmt.Sprintf("%d lines of the report were not nuclei results and were left out", report.Skipped))
	}

	if unplaced > 0 {
		in.Notes = append(in.Notes, fmt.Sprintf("%d results named no host, URL or address and were left out", unplaced))
	}

	if len(in.Nodes) == 0 {
		return response.BadRequest(c, "no result of the report names a target")
	}

	res, err := h.doImport(c, h.sketchID(c), in)
	if err != nil {
		return response.Internal(c, err)
	}

	if err := h.markSeen(c, res.written, &res.out); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, res.out)
}

// markSeen dates the findings of this report (last_seen) and the
// nuclei findings on the same targets that this report came without
// (missing_since, kept from the first report without them), so a hole
// since the last scan shows. A finding seen again loses the mark.
func (h Handler) markSeen(c fiber.Ctx, written []graphmodel.Node, out *importResponse) error {
	now := time.Now().UTC().Format(time.RFC3339)
	seen := map[string]bool{}
	targets := map[string]bool{}
	for _, n := range written {
		switch n.Type {
		case "finding":
			seen[n.ID] = true
		case "website", "domain", "ip":
			targets[n.ID] = true
		}
	}

	nodes, edges, err := h.Runtime.Store.Graph.Graph(c, h.sketchID(c))
	if err != nil {
		return err
	}

	byID := map[string]graphmodel.Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}

	var gone []string
	for _, e := range edges {
		f, ok := byID[e.TargetID]
		if e.Label != "HAS_FINDING" || !targets[e.SourceID] || !ok || f.Type != "finding" || seen[f.ID] || f.Properties["source"] != "nuclei" {
			continue
		}

		if f.Properties["missing_since"] != nil && f.Properties["missing_since"] != "" {
			continue
		}

		if _, err := h.Runtime.Store.Graph.UpdateNode(c, h.sketchID(c), f.ID, graphmodel.NodePatch{Properties: map[string]any{"missing_since": now}}); err != nil {
			return err
		}

		gone = append(gone, f.Label)
	}

	for id := range seen {
		if _, err := h.Runtime.Store.Graph.UpdateNode(c, h.sketchID(c), id, graphmodel.NodePatch{Properties: map[string]any{"last_seen": now, "missing_since": nil}}); err != nil {
			return err
		}
	}

	if len(gone) > 0 {
		shown := gone[:min(5, len(gone))]
		note := fmt.Sprintf("%d earlier findings on these targets are not in this report, marked missing: %s", len(gone), strings.Join(shown, ", "))
		if len(gone) > len(shown) {
			note += fmt.Sprintf(" and %d more", len(gone)-len(shown))
		}

		out.Errors = append(out.Errors, note)
	}

	return nil
}

// nucleiImport turns the results into entities and relationships, each
// target and CVE once however many results name it. unplaced counts the
// results that name nothing to hang from.
func nucleiImport(rep *nucleireport.Report) (in importInput, unplaced int) {
	seen := map[string]bool{}
	node := func(ref, typ string, props map[string]any, flag string) {
		if seen[ref] {
			return
		}

		seen[ref] = true
		in.Nodes = append(in.Nodes, importNode{ID: ref, Type: typ, Properties: props, Flag: flag})
	}

	linked := map[string]bool{}
	edge := func(from, to, label string) {
		if k := from + "|" + label + "|" + to; !linked[k] {
			linked[k] = true
			in.Edges = append(in.Edges, importEdge{Source: from, Target: to, Label: label})
		}
	}

	for _, r := range rep.Results {
		if hostOf(r) == "" && r.IP == "" {
			unplaced++

			continue
		}

		target := ""
		if host := hostOf(r); host != "" {
			if addr, err := netip.ParseAddr(host); err == nil {
				target = "ip:" + addr.String()
				node(target, "ip", map[string]any{"address": addr.String()}, "")
			} else {
				target = "domain:" + host
				node(target, "domain", map[string]any{"domain": host}, "")
				if addr, err := netip.ParseAddr(r.IP); err == nil {
					node("ip:"+addr.String(), "ip", map[string]any{"address": addr.String()}, "")
					edge(target, "ip:"+addr.String(), "RESOLVES_TO")
				}
			}
		}

		if site := originOf(r); site != "" && r.Type == "http" {
			node("site:"+site, "website", map[string]any{"url": site}, "")
			if target != "" {
				edge(target, "site:"+site, "HAS_WEBSITE")
			}

			target = "site:" + site
		}

		location := r.MatchedAt
		if location == "" {
			location = r.Host
		}

		finding := map[string]any{"title": r.Name, "rule": r.TemplateID, "location": location, "source": "nuclei"}
		if slices.Contains(types.Severities, r.Severity) {
			finding["severity"] = r.Severity
		}

		var evidence []string
		if r.Matcher != "" {
			evidence = append(evidence, "matcher: "+r.Matcher)
		}

		evidence = append(evidence, r.Extracted...)
		for k, v := range map[string]any{
			"evidence": strings.Join(evidence, "\n"), "description": r.Description,
			"references": r.References, "tags": r.Tags,
		} {
			if s, ok := v.(string); ok && s != "" {
				finding[k] = s
			}

			if l, ok := v.([]string); ok && len(l) > 0 {
				finding[k] = l
			}
		}

		if !r.Timestamp.IsZero() {
			finding["observed_at"] = r.Timestamp.Format(time.RFC3339)
		}

		// The same choice the type's key makes: the rule, else the title.
		ref := "finding:" + cmp.Or(r.TemplateID, r.Name) + "@" + location
		node(ref, "finding", finding, severityFlag[r.Severity])
		if target != "" {
			edge(target, ref, "HAS_FINDING")
		}

		for _, id := range r.CVEs {
			cve := map[string]any{"id": id}
			if r.CVSS > 0 {
				cve["cvss"] = r.CVSS
			}

			if len(r.CWEs) > 0 {
				cve["cwe"] = r.CWEs[0]
			}

			node("cve:"+id, "cve", cve, "")
			edge(ref, "cve:"+id, "REFERENCES")
		}
	}

	return in, unplaced
}

// hostOf is the name or address a result is about, without scheme,
// port or path.
func hostOf(r nucleireport.Result) string {
	for _, raw := range []string{r.Host, r.MatchedAt} {
		if raw == "" {
			continue
		}

		if strings.Contains(raw, "://") {
			if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
				return strings.ToLower(u.Hostname())
			}

			continue
		}

		if h, _, err := net.SplitHostPort(raw); err == nil {
			return strings.ToLower(h)
		}

		return strings.ToLower(strings.Trim(raw, "[]"))
	}

	return ""
}

// originOf is the site a result was on, scheme://host[:port], "" when
// the result names none.
func originOf(r nucleireport.Result) string {
	for _, raw := range []string{r.Host, r.MatchedAt} {
		if u, err := url.Parse(raw); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			return u.Scheme + "://" + strings.ToLower(u.Host)
		}
	}

	return ""
}
