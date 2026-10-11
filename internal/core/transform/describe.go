// Gosint, Copyright (c) 2026 YouSysAdmin

package transform

import (
	"fmt"
	"net/url"
	"strings"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// describedInputs is how many inputs the run line names.
const describedInputs = 5

// Describe is the line a run logs after its start: what it works on, the
// first inputs by label, and the parameters it runs with.
func Describe(meta Meta, inputs []graphmodel.Node, params, given map[string]string) string {
	noun := meta.Input
	if len(inputs) != 1 {
		noun += "s"
	}

	names := make([]string, 0, describedInputs)
	for _, n := range inputs[:min(len(inputs), describedInputs)] {
		names = append(names, n.Label)
	}

	line := fmt.Sprintf("on %d %s: %s", len(inputs), noun, strings.Join(names, ", "))
	if more := len(inputs) - describedInputs; more > 0 {
		line += fmt.Sprintf(" and %d more", more)
	}

	if p := DescribeParams(meta, params, given); p != "" {
		line += ". " + p
	}

	return line
}

// DescribeParams lists name=value for every parameter in the order the
// transform declares them. A secret is named, its value never shown.
func DescribeParams(meta Meta, params, given map[string]string) string {
	var parts []string
	for _, p := range meta.Params {
		if p.Type == ParamSecret {
			key := given[p.Name]
			if key == "" {
				key = p.Name
			}

			state := "secret " + key
			if params[p.Name] == "" {
				state += " (not stored)"
			}

			parts = append(parts, p.Name+"="+state)

			continue
		}

		if v, ok := params[p.Name]; ok {
			if p.Type == ParamURL {
				v = loggedURL(v)
			}

			parts = append(parts, p.Name+"="+v)
		}
	}

	return strings.Join(parts, ", ")
}

// loggedURL is a URL param as the log shows it: without the user, the
// password, the query and the fragment, where a key would ride.
func loggedURL(v string) string {
	u, err := url.Parse(v)
	if err != nil || u.Host == "" {
		return "(a URL)"
	}

	u.User, u.RawQuery, u.Fragment, u.RawFragment, u.ForceQuery = nil, "", "", "", false

	return u.String()
}
