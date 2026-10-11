// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/domain/flow"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	scanmodel "github.com/yousysadmin/gosint/internal/models/scan"
)

// The model may RUN transforms, one or a chain composed on the spot, when
// the person asking could: the server launches the scan, waits for it
// and hands the model what it wrote. A run that touches the target or
// spends a key is not launched but proposed as a card, so the person
// decides.

// The kinds of run.
const (
	ActRunTransform = "run_transform"
	ActRunFlow      = "run_flow"
)

const (
	// runTimeout bounds the wait for one run inside an answer. The
	// scan goes on past it and the model is told so.
	runTimeout = 4 * time.Minute

	// writtenShown bounds the entities a run's result lists.
	writtenShown = 50

	// catalogChars bounds the transform list in the prompt.
	catalogChars = 6000
)

// params as a tool schema: a bare object reads as an EMPTY one on a
// server that turns the schema into a grammar.
var paramsSchema = map[string]any{"type": "object", "description": "parameter name to value",
	"additionalProperties": map[string]any{"type": []string{"string", "number", "boolean"}}}

// runTool is the running tool.
var runTool = llm.Tool{
	Name: "run",
	Description: "Run transforms on entities of the sketch, which adds what they find to the graph: run_transform runs one, " +
		"run_flow runs a chain where each step takes what its parent step wrote. The result, with the entities written, " +
		"comes back as the next message. Only the transforms listed in the system prompt exist.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind":      map[string]any{"type": "string", "enum": []string{ActRunTransform, ActRunFlow}},
			"transform": map[string]any{"type": "string", "description": "run_transform: the transform name"},
			"params":    paramsSchema,
			"node_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
				"description": "the entities to run on, the selected ones when left out"},
			"input": map[string]any{"type": "string", "description": "run_flow: the entity type the flow starts from"},
			"steps": map[string]any{"type": "array", "description": "run_flow: the steps",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":        map[string]any{"type": "string"},
						"transform": map[string]any{"type": "string"},
						"after":     map[string]any{"type": "string", "description": "the parent step id, or input for the first"},
						"params":    paramsSchema,
					},
					"required": []string{"id", "transform", "after"},
				}},
			"reason": map[string]any{"type": "string", "description": "one line on why"},
		},
		"required": []string{"kind"},
	},
}

// The two ways to run, told to the model by what it has.
const (
	runShape = "A run is kind run_transform (transform, params, node_ids, reason) or run_flow (input, steps, node_ids, " +
		"reason). node_ids left out means the selected entities. A flow step is {id, transform, after, params}: after " +
		"names the parent step, or input for the first, and a step is fed what its parent WROTE, so its transform must " +
		"take the parent's output type. Example for a selected domain: {\"kind\": \"run_flow\", \"input\": \"domain\", " +
		"\"steps\": [{\"id\": \"subs\", \"transform\": \"domain_to_subdomains\", \"after\": \"input\"}, " +
		"{\"id\": \"ips\", \"transform\": \"domain_to_ip\", \"after\": \"subs\"}, " +
		"{\"id\": \"clean\", \"transform\": \"ip_drop_cdn\", \"after\": \"ips\"}, " +
		"{\"id\": \"ports\", \"transform\": \"ip_to_ports\", \"after\": \"clean\"}]}. " +
		"A run writes to the sketch itself, so never propose add_entity for what a run found. Use only the transforms " +
		"listed under \"Transforms you may run\". When the person asks for work a transform does, run it instead of " +
		"describing how. "

	runToolPrompt = "Use the run tool to run transforms and flows. " + runShape

	runBlockPrompt = "To run transforms, put the run in the " + actionsFence + " block as one of the actions, the same " +
		"way as a lookup. " + runShape

	readOnlyPrompt = "The person's role on this investigation is read only, so transforms cannot be run from this " +
		"chat and changes can only be suggested. "
)

// runStep is a flow step as the model writes it, params of any type.
type runStep struct {
	ID        string         `json:"id"`
	Transform string         `json:"transform"`
	After     string         `json:"after"`
	Params    map[string]any `json:"params,omitzero"`
}

// isRun says the action runs something rather than reads.
func (a Action) isRun() bool { return a.Kind == ActRunTransform || a.Kind == ActRunFlow }

// runLabel is how the run is shown while it runs.
func (a Action) runLabel() string {
	n := len(a.NodeIDs)
	on := fmt.Sprintf("on %d entities", n)
	if n == 1 {
		on = "on 1 entity"
	}

	if a.Kind == ActRunFlow {
		return fmt.Sprintf("Running a %d-step flow %s", len(a.Steps), on)
	}

	return "Running " + a.Transform + " " + on
}

// stringParams is the model's parameter values as the scan stores
// them, strings.
func stringParams(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = fmt.Sprint(v)
	}

	return out
}

// needsConfirm says a run is the person's to start: it contacts the
// target, spends a key of theirs, or sends their data somewhere: a
// source other than the default, a required secret, a URL the model
// chose, or an automation handing entities to another system.
func needsConfirm(meta transform.Meta, params map[string]string) bool {
	if meta.Category == "Automation" {
		return true
	}

	for _, p := range meta.Params {
		// A source other than the default is the active or the paid
		// one: nmap against the target, a paid API.
		if p.Name == "source" && params["source"] != "" && params["source"] != p.Default {
			return true
		}

		if p.Type == transform.ParamSecret && p.Required {
			return true
		}

		// A page the model read could name the destination.
		if p.Type == transform.ParamURL && params[p.Name] != "" {
			return true
		}
	}

	return false
}

// prepareRun turns a run into a scan ready to queue, after the checks a
// launch from the console makes. A run the person must confirm comes
// back as a change instead, unless confirmed says they already did.
func (h Handler) prepareRun(ctx context.Context, sketchID, owner string, a Action, confirmed bool) (*scanmodel.Scan, *Change, error) {
	rt := h.Runtime
	r := newResolverFrom(ctx, h, sketchID)
	nodeIDs := make([]string, 0, len(a.NodeIDs))
	for _, ref := range a.NodeIDs {
		id, err := r.resolve(ref)
		if err != nil {
			return nil, nil, err
		}

		if id != "" && !slices.Contains(nodeIDs, id) {
			nodeIDs = append(nodeIDs, id)
		}
	}

	if len(nodeIDs) == 0 {
		return nil, nil, errors.New("no entities to run on: select some or give node_ids")
	}

	secret := func(ref string) (string, bool, error) { return vaultkey.Secret(ctx, rt, owner, ref) }
	switch a.Kind {
	case ActRunTransform:
		e, ok := rt.Transforms.Get(strings.TrimSpace(a.Transform))
		if !ok {
			return nil, nil, fmt.Errorf("no transform %q, use a name from the list", a.Transform)
		}

		meta, params := e.Meta(), stringParams(a.Params)
		if _, err := transform.ResolveParams(meta.Params, params, secret); err != nil {
			return nil, nil, err
		}

		if err := h.usable(ctx, sketchID, nodeIDs, meta.Input); err != nil {
			return nil, nil, err
		}

		if !confirmed && needsConfirm(meta, params) {
			return nil, &Change{Kind: KindRun, Transform: meta.Name, NodeIDs: nodeIDs, Params: params, Reason: a.Reason}, nil
		}

		return &scanmodel.Scan{ID: ids.New(), SketchID: sketchID, OwnerID: owner, Kind: transform.ScanKind, Name: meta.Name,
			Input: transform.Input{NodeIDs: nodeIDs, Params: params}.InputMap()}, nil, nil
	case ActRunFlow:
		def := flowmodel.Definition{Input: strings.TrimSpace(a.Input), Steps: make([]flowmodel.Step, 0, len(a.Steps))}
		for _, s := range a.Steps {
			def.Steps = append(def.Steps, flowmodel.Step{ID: s.ID, Transform: strings.TrimSpace(s.Transform),
				After: strings.TrimSpace(s.After), Params: stringParams(s.Params)})
		}

		// A model writes an entity id or a label where the type goes, so
		// anything but a known type is taken from the first step.
		if _, known := types.Builtin().Get(def.Input); !known {
			def.Input = h.rootInput(def)
		}

		if err := coreflow.Check(def, rt.Transforms); err != nil {
			return nil, nil, err
		}

		confirm := false
		for _, s := range def.Steps {
			e, _ := rt.Transforms.Get(s.Transform)
			meta := e.Meta()
			if _, err := transform.ResolveParams(meta.Params, s.Params, secret); err != nil {
				return nil, nil, fmt.Errorf("step %s: %w", s.ID, err)
			}

			confirm = confirm || needsConfirm(meta, s.Params)
		}

		if err := h.usable(ctx, sketchID, nodeIDs, def.Input); err != nil {
			return nil, nil, err
		}

		if !confirmed && confirm {
			return nil, &Change{Kind: KindRun, Input: def.Input, Steps: def.Steps, NodeIDs: nodeIDs, Reason: a.Reason}, nil
		}

		return flow.InlineScan(sketchID, owner, def, nodeIDs), nil, nil
	default:
		return nil, nil, fmt.Errorf("unknown run %q", a.Kind)
	}
}

// rootInput is the type the first step's transform takes, for a model
// that left the flow's input out.
func (h Handler) rootInput(def flowmodel.Definition) string {
	for _, s := range def.Steps {
		if s.After != coreflow.Root {
			continue
		}

		if e, ok := h.Runtime.Transforms.Get(s.Transform); ok {
			return e.Meta().Input
		}
	}

	return ""
}

// usable refuses a run with nothing of the input type among the nodes.
func (h Handler) usable(ctx context.Context, sketchID string, nodeIDs []string, inputType string) error {
	n, err := scan.UsableNodes(ctx, h.Runtime, sketchID, nodeIDs, inputType)
	if err != nil {
		return err
	}

	if n == 0 {
		return fmt.Errorf("none of the entities is a %s, which this run takes", inputType)
	}

	return nil
}

// run performs one run for the model: queued, waited for, and rendered
// as text, or turned into a change to confirm. progress hears how the
// wait goes.
func (h Handler) run(ctx context.Context, sketchID, owner string, a Action, progress func(string)) (string, *Change) {
	sc, confirm, err := h.prepareRun(ctx, sketchID, owner, a, false)
	if err != nil {
		return "error: " + err.Error(), nil
	}

	if confirm != nil {
		return "This run contacts the target or spends a key, so it was not started: it is shown to the person as a " +
			"card to confirm. Say so, and do not ask for it again.", confirm
	}

	wctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()

	start := time.Now()
	done, err := scan.Wait(wctx, h.Runtime, sc, func() error { return scan.Enqueue(ctx, h.Runtime, sc) },
		func(elapsed time.Duration) {
			progress(fmt.Sprintf("Still running %s, %ds", sc.Name, int(elapsed.Seconds())))
		})
	if done == nil {
		done = sc
	}

	return h.runResult(ctx, sketchID, done, err, time.Since(start)), nil
}

// runResult renders a settled scan for the model: what happened, the
// counts, each step of a flow, and the entities written.
func (h Handler) runResult(ctx context.Context, sketchID string, sc *scanmodel.Scan, waitErr error, elapsed time.Duration) string {
	var b strings.Builder
	elapsed = elapsed.Round(time.Second)
	if waitErr != nil || !sc.Status.Final() {
		fmt.Fprintf(&b, "%s is still running after %s. It goes on in the background and its result shows in the "+
			"Scans drawer, tell the person so.\n", sc.Name, elapsed)

		return b.String()
	}

	fmt.Fprintf(&b, "%s %s in %s", sc.Name, sc.Status, elapsed)
	if sc.Error != "" {
		b.WriteString(": " + sc.Error)
	}

	b.WriteString("\n")
	if n, ok := sc.Details["nodes"]; ok {
		fmt.Fprintf(&b, "Wrote %v entities and %v relationships.\n", n, sc.Details["edges"])
	}

	if steps, ok := sc.Details["steps"].(map[string]any); ok {
		for _, id := range slices.Sorted(maps.Keys(steps)) {
			b.WriteString(h.describeStep(sc, id, steps[id]))
		}
	}

	raw, _ := sc.Details["written_ids"].([]any)
	written := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			written = append(written, s)
		}
	}

	if len(written) == 0 {
		b.WriteString("Nothing new was written.\n")

		return b.String()
	}

	shown := written
	if len(shown) > writtenShown {
		shown = shown[:writtenShown]
	}

	nodes, err := h.Runtime.Store.Graph.NodesIn(ctx, sketchID, shown)
	if err != nil {
		return b.String()
	}

	fmt.Fprintf(&b, "Entities written (%d of %d), already in the sketch:\n", len(nodes), len(written))
	for _, n := range nodes {
		b.WriteString(describeNode(n))
	}

	b.WriteString("This is the whole outcome, the sketch is up to date: answer from it, do not run the same thing again.\n")

	return b.String()
}

// describeStep renders one step of a flow for the model. A filter's
// numbers are said as kept and dropped, the counts alone read as a
// step that found nothing.
func (h Handler) describeStep(sc *scanmodel.Scan, id string, summary any) string {
	counts, ok := summary.(map[string]any)
	if !ok {
		return fmt.Sprintf("- step %s: %v\n", id, summary)
	}

	name := stepTransform(sc, id)
	inputs, _ := counts["inputs"].(float64)
	nodes, _ := counts["nodes"].(float64)
	edges, _ := counts["edges"].(float64)
	if e, found := h.Runtime.Transforms.Get(name); found && e.Meta().Category == filterCategory {
		kept, ok := counts["passed"].(float64)
		if !ok {
			kept = nodes
		}

		return fmt.Sprintf("- step %s (%s, a filter): %d in, %d kept and passed on, %d dropped and marked in the sketch\n",
			id, name, int(inputs), int(kept), int(inputs-kept))
	}

	return fmt.Sprintf("- step %s (%s): %d in, wrote %d entities and %d relationships\n", id, name, int(inputs), int(nodes), int(edges))
}

// filterCategory is the category of the narrowing transforms.
const filterCategory = "Filter"

// stepTransform is the transform a step of an inline flow scan runs, read
// back from the definition the scan carries.
func stepTransform(sc *scanmodel.Scan, id string) string {
	raw, _ := sc.Input["definition"].(map[string]any)
	steps, _ := raw["steps"].([]any)
	for _, v := range steps {
		s, _ := v.(map[string]any)
		if s["id"] == id {
			name, _ := s["transform"].(string)

			return name
		}
	}

	return "?"
}

// transformCatalog lists the transforms the model may run: those taking a
// selected type and those taking what one of them produces, so a chain
// from the selection can be composed. With nothing selected, all of
// them. Templates are left out, they are the person's own.
func transformCatalog(reg *transform.Registry, nodes []graphmodel.Node) string {
	want := map[string]bool{}
	for _, n := range nodes {
		want[n.Type] = true
	}

	all := reg.For("")
	hop := maps.Clone(want)
	for _, e := range all {
		if m := e.Meta(); want[m.Input] {
			hop[m.Output] = true
		}
	}

	var lines []string
	for _, e := range all {
		m := e.Meta()
		if len(want) > 0 && m.Input != "" && !hop[m.Input] {
			continue
		}

		in := m.Input
		if in == "" {
			in = "any"
		}

		out := m.Output
		if out == "" {
			out = "what its fields name"
		}

		line := fmt.Sprintf("%s: %s -> %s. %s", m.Name, in, out, m.Description)
		if len(m.Params) > 0 {
			names := make([]string, 0, len(m.Params))
			for _, p := range m.Params {
				name := p.Name
				if p.Default != "" {
					name += "=" + p.Default
				}

				names = append(names, name)
			}

			line += " (params: " + strings.Join(names, ", ") + ")"
		}

		lines = append(lines, line)
	}

	out := strings.Join(lines, "\n")
	if len(out) > catalogChars {
		out = out[:catalogChars] + "\n..."
	}

	return out
}
