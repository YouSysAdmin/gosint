// Gosint, Copyright (c) 2026 YouSysAdmin

package flow_test

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms"
)

func TestCheck(t *testing.T) {
	reg := transform.NewRegistry(transforms.All()...)
	ok := flow.Definition{Input: "email", Steps: []flow.Step{
		{ID: "b", Transform: "domain_to_root_domain", After: "a"},
		{ID: "a", Transform: "email_to_domain", After: flow.Root},
		{ID: "c", Transform: "email_to_username", After: flow.Root},
	}}

	if err := flow.Check(ok, reg); err != nil {
		t.Fatalf("a valid flow was refused: %v", err)
	}

	order, _ := flow.Order(ok)
	if order[0].ID != "a" || order[2].ID != "b" {
		t.Errorf("order = %+v, parents must come first", order)
	}

	for name, tc := range map[string]struct {
		def  flow.Definition
		want string
	}{
		"wrong type": {flow.Definition{Input: "email", Steps: []flow.Step{
			{ID: "a", Transform: "ip_to_asn", After: flow.Root}}}, "takes ip"},
		"unknown transform": {flow.Definition{Input: "email", Steps: []flow.Step{
			{ID: "a", Transform: "nope", After: flow.Root}}}, "no transform"},
		"unknown parent": {flow.Definition{Input: "email", Steps: []flow.Step{
			{ID: "a", Transform: "email_to_domain", After: "ghost"}}}, "not a step"},
		"cycle": {flow.Definition{Input: "domain", Steps: []flow.Step{
			{ID: "a", Transform: "domain_to_root_domain", After: "b"},
			{ID: "b", Transform: "domain_to_root_domain", After: "a"}}}, "cycle"},
		"bad param": {flow.Definition{Input: "cidr", Steps: []flow.Step{
			{ID: "a", Transform: "cidr_to_ips", After: flow.Root, Params: map[string]string{"limit": "lots"}}}}, "number"},
		"duplicate id": {flow.Definition{Input: "email", Steps: []flow.Step{
			{ID: "a", Transform: "email_to_domain", After: flow.Root},
			{ID: "a", Transform: "email_to_username", After: flow.Root}}}, "twice"},
	} {
		err := flow.Check(tc.def, reg)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to mention %q", name, err, tc.want)
		}
	}
}
