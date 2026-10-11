// Gosint, Copyright (c) 2026 YouSysAdmin

// Package flow chains transforms: a tree of steps from an input entity
// type, each step fed the nodes its parent step wrote.
//
// A definition is checked when saved, so a step whose input type its
// parent cannot produce is refused there rather than failing at run
// time.
package flow

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/yousysadmin/gosint/internal/core/transform"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
)

// Root is the parent name of a step fed the flow's input nodes.
const Root = flowmodel.Root

// Definition and Step are the stored shapes.
type (
	Definition = flowmodel.Definition
	Step       = flowmodel.Step
)

// maxSteps bounds a flow's size.
const maxSteps = 64

var stepIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Check refuses a definition that cannot run: unknown transforms,
// unknown parents, cycles, or a step whose input its parent cannot
// produce. Params are checked for names and values, not secrets, which
// are the launcher's.
func Check(d Definition, reg *transform.Registry) error {
	if d.Input == "" {
		return errors.New("flow: input type is required")
	}

	if len(d.Steps) == 0 || len(d.Steps) > maxSteps {
		return fmt.Errorf("flow: between 1 and %d steps", maxSteps)
	}

	output := map[string]string{Root: d.Input}
	parent := map[string]string{}
	for _, s := range d.Steps {
		if !stepIDRe.MatchString(s.ID) || s.ID == Root {
			return fmt.Errorf("flow: step id %q is not valid", s.ID)
		}

		if _, dup := parent[s.ID]; dup {
			return fmt.Errorf("flow: step id %s is used twice", s.ID)
		}

		e, ok := reg.Get(s.Transform)
		if !ok {
			return fmt.Errorf("flow: step %s: no transform %q", s.ID, s.Transform)
		}

		output[s.ID] = e.Meta().Output
		parent[s.ID] = s.After
	}

	for _, s := range d.Steps {
		from, ok := output[s.After]
		if !ok {
			return fmt.Errorf("flow: step %s comes after %q, which is not a step", s.ID, s.After)
		}

		e, _ := reg.Get(s.Transform)
		// A parent producing "" (fields_to_entities draws many types)
		// feeds any step, the run filters by type.
		if in := e.Meta().Input; in != "" && from != "" && in != from {
			return fmt.Errorf("flow: step %s runs %s, which takes %s, after a step producing %s", s.ID, s.Transform, in, from)
		}

		if err := checkParams(e.Meta().Params, s.Params); err != nil {
			return fmt.Errorf("flow: step %s: %w", s.ID, err)
		}
	}

	if _, err := Order(d); err != nil {
		return err
	}

	return nil
}

// checkParams checks names and values, leaving secrets unopened.
func checkParams(params []transform.Param, given map[string]string) error {
	_, err := transform.ResolveParams(params, given, func(string) (string, bool, error) { return "", true, nil })

	return err
}

// Order lists the steps parents first, failing on a cycle.
func Order(d Definition) ([]Step, error) {
	done := map[string]bool{Root: true}
	var out []Step
	for len(out) < len(d.Steps) {
		progressed := false
		for _, s := range d.Steps {
			if !done[s.ID] && done[s.After] {
				done[s.ID] = true
				out = append(out, s)
				progressed = true
			}
		}

		if !progressed {
			return nil, errors.New("flow: the steps form a cycle")
		}
	}

	return out, nil
}
