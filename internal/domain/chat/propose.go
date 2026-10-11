// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/eventbus"
	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/scan"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// The model does not write to the graph. It PROPOSES: a tool call, or
// for a model with no tool support a fenced block in its answer, both
// turned into Change values the person applies with a click. A
// proposal lives in the stored answer as the fenced block, so history
// shows it again.

// The kinds of change.
const (
	KindSetProperties = "set_properties"
	KindAddEntity     = "add_entity"
	KindLink          = "link"

	// KindRun is a run the model asked for that the person must start:
	// one transform, or a flow of steps (run.go).
	KindRun = "run"
)

// blockFence opens the fenced block a proposal travels in.
const blockFence = "```gosint-changes"

// maxChanges bounds one proposal.
const maxChanges = 50

// proposeTool is the one tool the chat offers.
var proposeTool = llm.Tool{
	Name: "propose_changes",
	Description: "Propose changes to the sketch's graph for the person to apply: fill properties of a selected entity, " +
		"add a new entity linked to one, or link two entities. Use only entity types and property names you were shown. " +
		"Propose only what the evidence supports, never a guess.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"changes": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"kind":    map[string]any{"type": "string", "enum": []string{KindSetProperties, KindAddEntity, KindLink}},
						"node_id": map[string]any{"type": "string", "description": "set_properties: the id of the entity to fill, as shown in brackets"},
						// additionalProperties spelled out: a server that turns the
						// schema into a grammar reads a bare object as an EMPTY one.
						"properties": map[string]any{"type": "object", "description": "set_properties and add_entity: property name to value",
							"additionalProperties": map[string]any{"type": []string{"string", "number", "boolean"}}},
						"type":     map[string]any{"type": "string", "description": "add_entity: the entity type"},
						"link_to":  map[string]any{"type": "string", "description": "add_entity: an existing entity id to link the new one to"},
						"relation": map[string]any{"type": "string", "description": "add_entity: the relationship label, UPPER_SNAKE_CASE"},
						"source":   map[string]any{"type": "string", "description": "link: entity id"},
						"target":   map[string]any{"type": "string", "description": "link: entity id"},
						"label":    map[string]any{"type": "string", "description": "link: the relationship label"},
						"reason":   map[string]any{"type": "string", "description": "one line on why"},
					},
					"required": []string{"kind"},
				},
			},
		},
		"required": []string{"changes"},
	},
}

// The two ways to propose, told to the model by what it has.
const (
	proposeShape = "A change is kind set_properties (node_id, properties, reason), add_entity (type, properties, " +
		"link_to, relation, reason) or link (source, target, label, reason). Refer to entities by the id shown in " +
		"brackets. Use only the property names listed for the type. Propose only what the evidence supports."

	proposeToolPrompt = "You may propose changes to the graph with the propose_changes tool. " + proposeShape

	proposeBlockPrompt = "You may propose changes to the graph: end your answer with a fenced block that starts " +
		"with " + blockFence + " and holds {\"changes\": [...]}. " + proposeShape
)

// Change is one proposed edit. An entity reference (node_id, link_to,
// source, target) is an id, or a label when the model used the name it
// read instead of the id: apply resolves both.
type Change struct {
	Kind       string         `json:"kind"                validate:"required,oneof=set_properties add_entity link run"`
	NodeID     string         `json:"node_id,omitzero"    validate:"max=200"`
	Properties map[string]any `json:"properties,omitzero"`
	Type       string         `json:"type,omitzero"       validate:"max=60"`
	LinkTo     string         `json:"link_to,omitzero"    validate:"max=200"`
	Relation   string         `json:"relation,omitzero"   validate:"max=128"`
	Source     string         `json:"source,omitzero"     validate:"max=200"`
	Target     string         `json:"target,omitzero"     validate:"max=200"`
	Label      string         `json:"label,omitzero"      validate:"max=128"`
	Reason     string         `json:"reason,omitzero"     validate:"max=500"`

	// A run: one transform with params, or a flow from input over steps,
	// on node_ids.
	Transform string            `json:"transform,omitzero" validate:"max=100"`
	Params    map[string]string `json:"params,omitzero"   validate:"max=20"`
	NodeIDs   []string          `json:"node_ids,omitzero" validate:"max=1000,dive,uuid"`
	Input     string            `json:"input,omitzero"    validate:"max=60"`
	Steps     []flowmodel.Step  `json:"steps,omitzero"    validate:"max=64"`
}

// proposal is what the tool or the block carries.
type proposal struct {
	Changes []Change `json:"changes"`
}

// changesOf turns the model's calls and any block in its text into
// changes, and returns the text with the block taken out.
func changesOf(text string, calls []llm.ToolCall) (string, []Change) {
	var out []Change
	for _, c := range calls {
		if c.Name != proposeTool.Name {
			continue
		}

		var p proposal
		if err := json.Unmarshal([]byte(c.Arguments), &p); err == nil {
			out = append(out, p.Changes...)
		}
	}

	text, inline := splitBlock(text)
	out = append(out, inline...)
	if len(out) > maxChanges {
		out = out[:maxChanges]
	}

	return text, out
}

// splitBlock takes the proposal block out of the text: the named fence,
// or any fenced JSON that carries a changes key, since a model does not
// always keep the fence name it was told.
func splitBlock(text string) (string, []Change) {
	var out []Change
	text = takeBlocks(text, func(body []byte) bool {
		var p proposal
		if err := json.Unmarshal(body, &p); err != nil || p.Changes == nil {
			return false
		}

		out = append(out, p.Changes...)

		return true
	})

	return text, out
}

// takeBlocks hands every fenced code block to keep and removes the ones
// it takes, closing the text up around them.
func takeBlocks(text string, keep func(body []byte) bool) string {
	for from := 0; ; {
		start := strings.Index(text[from:], "```")
		if start < 0 {
			return text
		}

		start += from
		nl := strings.Index(text[start:], "\n")
		if nl < 0 {
			return text
		}

		bodyStart := start + nl + 1
		end := strings.Index(text[bodyStart:], "```")
		if end < 0 {
			return text
		}

		end += bodyStart
		body := strings.TrimSpace(text[bodyStart:end])
		if !strings.HasPrefix(body, "{") || !keep([]byte(body)) {
			from = end + 3

			continue
		}

		after := text[end+3:]
		text = strings.TrimSpace(strings.TrimRight(text[:start], "\n") + "\n\n" + strings.TrimLeft(after, "\n"))
		from = 0
	}
}

// withBlock appends the changes to the answer as the fenced block.
func withBlock(text string, changes []Change) string {
	if len(changes) == 0 {
		return text
	}

	b, err := json.Marshal(proposal{Changes: changes})
	if err != nil {
		return text
	}

	return strings.TrimSpace(text) + "\n\n" + blockFence + "\n" + string(b) + "\n```"
}

// catalog lists the fields of the types among the selected entities,
// so the model proposes properties that exist.
func catalog(nodes []graphmodel.Node) string {
	seen := map[string]bool{}
	var lines []string
	for _, n := range nodes {
		t, ok := types.Builtin().Get(n.Type)
		if !ok || seen[n.Type] {
			continue
		}

		seen[n.Type] = true
		names := make([]string, 0, len(t.Fields))
		for _, f := range t.Fields {
			names = append(names, f.Name)
		}

		lines = append(lines, n.Type+": "+strings.Join(names, ", "))
	}

	slices.Sort(lines)

	return strings.Join(lines, "\n")
}

// Apply writes the changes the person accepted. Each goes through the
// entity types as a hand edit would, one refused is reported and the
// rest go on.
func (h Handler) Apply(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[applyInput](c)
	if !ok {
		return resp
	}

	sketchID, owner := access.Sketch(c).ID, access.User(c).ID
	nodes, _, err := h.Runtime.Store.Graph.Graph(c, sketchID)
	if err != nil {
		return response.Internal(c, err)
	}

	byRef := newResolver(nodes)
	u := access.User(c)
	origin := graphmodel.Origin{User: u.ID, Name: u.DisplayName(), Via: "chat", At: time.Now().UTC()}
	out := applyResponse{Errors: []string{}, Nodes: []graphmodel.Node{}}
	for i, ch := range in.Changes {
		if ch.Kind == KindRun {
			// The person confirmed the run the model asked for: it is
			// queued as a launch from the console would be, not waited for.
			if err := h.launch(c, sketchID, owner, ch); err != nil {
				out.Errors = append(out.Errors, fmt.Sprintf("change %d: %v", i+1, err))

				continue
			}

			out.Applied++
			out.Launched++

			continue
		}

		resolved, err := byRef.change(ch)
		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("change %d: %v", i+1, err))

			continue
		}

		n, err := h.apply(c, sketchID, resolved, origin)
		if n != nil {
			byRef.add(*n)
		}

		if err != nil {
			out.Errors = append(out.Errors, fmt.Sprintf("change %d: %v", i+1, err))

			continue
		}

		out.Applied++
		if n != nil {
			out.Nodes = append(out.Nodes, *n)
		}
	}

	if out.Applied > out.Launched {
		h.Runtime.Events.Publish(eventbus.Event{Type: eventbus.TypeGraph, SketchID: sketchID,
			Data: eventbus.GraphChange{By: u.ID, Name: u.DisplayName(), Kind: "assistant"}, At: time.Now().UTC()})
	}

	return response.Success(c, out)
}

var errNoNode = errors.New("not an entity of this sketch")

// launch queues a confirmed run.
func (h Handler) launch(ctx context.Context, sketchID, owner string, ch Change) error {
	a := Action{Kind: ActRunTransform, Transform: ch.Transform, NodeIDs: ch.NodeIDs, Reason: ch.Reason}
	for k, v := range ch.Params {
		if a.Params == nil {
			a.Params = map[string]any{}
		}

		a.Params[k] = v
	}

	if len(ch.Steps) > 0 {
		a.Kind, a.Input = ActRunFlow, ch.Input
		for _, s := range ch.Steps {
			step := runStep{ID: s.ID, Transform: s.Transform, After: s.After}
			for k, v := range s.Params {
				if step.Params == nil {
					step.Params = map[string]any{}
				}

				step.Params[k] = v
			}

			a.Steps = append(a.Steps, step)
		}
	}

	sc, _, err := h.prepareRun(ctx, sketchID, owner, a, true)
	if err != nil {
		return err
	}

	return scan.Enqueue(ctx, h.Runtime, sc)
}

// coerce types the model's values as the fields say: a number written
// as "1,234" or "12k" into an int, "true" into a bool. A model writes
// what it read, and what it read was text.
func coerce(typ string, props map[string]any) map[string]any {
	out := make(map[string]any, len(props))
	t, ok := types.Builtin().Get(typ)
	kinds := map[string]types.Kind{}
	if ok {
		for _, f := range t.Fields {
			kinds[f.Name] = f.Kind
		}
	}

	for k, v := range props {
		s, isString := v.(string)
		if !isString {
			out[k] = v

			continue
		}

		switch kinds[k] {
		case types.KindInt:
			if n, ok := parseCount(s); ok {
				out[k] = n

				continue
			}
		case types.KindFloat:
			if f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64); err == nil {
				out[k] = f

				continue
			}
		case types.KindBool:
			if b, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(s))); err == nil {
				out[k] = b

				continue
			}
		}

		out[k] = v
	}

	return out
}

// parseCount reads a count as sites print it: 1234, "1,234", "1.2k",
// "3.4M", "1 234".
func parseCount(s string) (int64, bool) {
	s = strings.ToLower(strings.NewReplacer(",", "", " ", "", "\u00a0", "").Replace(strings.TrimSpace(s)))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}

	return int64(f * float64(mult)), true
}

// relation is a relationship label as the graph spells them,
// UPPER_SNAKE_CASE, RELATED_TO when the model gave none.
func relation(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "RELATED_TO"
	}

	return strings.ToUpper(strings.NewReplacer(" ", "_", "-", "_").Replace(s))
}

// resolver turns the model's entity references into ids: an id passes,
// a label is looked up, case folded, with the punctuation a model
// leaves behind trimmed. A label two entities share names neither.
type resolver struct {
	ids    map[string]bool
	labels map[string][]graphmodel.Node
}

func newResolver(nodes []graphmodel.Node) *resolver {
	r := &resolver{ids: map[string]bool{}, labels: map[string][]graphmodel.Node{}}
	for _, n := range nodes {
		r.add(n)
	}

	return r
}

func (r *resolver) add(n graphmodel.Node) {
	r.ids[n.ID] = true
	key := strings.ToLower(n.Label)
	if !slices.ContainsFunc(r.labels[key], func(m graphmodel.Node) bool { return m.ID == n.ID }) {
		r.labels[key] = append(r.labels[key], n)
	}
}

// resolve is the id ref names, ref itself when it names nothing known,
// and an error when it is a label several entities carry.
func (r *resolver) resolve(ref string) (string, error) {
	ref = strings.Trim(strings.TrimSpace(ref), ",;. \t\"'")
	if ref == "" || r.ids[ref] {
		return ref, nil
	}

	switch found := r.labels[strings.ToLower(ref)]; len(found) {
	case 0:
		return ref, nil
	case 1:
		return found[0].ID, nil
	default:
		kinds := make([]string, 0, len(found))
		for _, n := range found {
			kinds = append(kinds, n.Type+" ["+n.ID+"]")
		}

		return "", fmt.Errorf("%q names %d entities (%s), give the id", ref, len(found), strings.Join(kinds, ", "))
	}
}

// change resolves every reference of ch.
func (r *resolver) change(ch Change) (Change, error) {
	var errs []error
	for _, ref := range []*string{&ch.NodeID, &ch.LinkTo, &ch.Source, &ch.Target} {
		id, err := r.resolve(*ref)
		errs = append(errs, err)
		*ref = id
	}

	return ch, errors.Join(errs...)
}

func (h Handler) apply(ctx context.Context, sketchID string, ch Change, origin graphmodel.Origin) (*graphmodel.Node, error) {
	graph := h.Runtime.Store.Graph
	switch ch.Kind {
	case KindSetProperties:
		if ch.NodeID == "" || len(ch.Properties) == 0 {
			return nil, errors.New("set_properties needs node_id and properties")
		}

		cur, err := graph.Node(ctx, sketchID, ch.NodeID)
		if err != nil || cur == nil {
			return nil, errors.Join(err, errNoNode)
		}

		// The whole merged set runs through the type like a hand edit:
		// a value the type refuses is refused here, the rest is stored
		// normalized, and a changed main field moves the key and label.
		reg := types.Builtin()
		merged := mergeProps(cur.Properties, coerce(cur.Type, ch.Properties))
		label, err := reg.Build(cur.Type, merged)
		if err != nil {
			return nil, err
		}

		key := reg.Key(cur.Type, merged)

		return graph.UpdateNode(ctx, sketchID, ch.NodeID, graphmodel.NodePatch{Key: &key, Label: &label, Properties: merged})
	case KindAddEntity:
		typ := strings.ToLower(strings.TrimSpace(ch.Type))
		reg := types.Builtin()
		if _, ok := reg.Get(typ); !ok {
			return nil, fmt.Errorf("unknown entity type %q", ch.Type)
		}

		props := coerce(typ, ch.Properties)
		label, err := reg.Build(typ, props)
		if err != nil {
			return nil, err
		}

		written, err := graph.UpsertNodes(ctx, sketchID, []graphmodel.Node{{Type: typ, Key: reg.Key(typ, props), Label: label, Properties: props,
			Metadata: map[string]any{graphmodel.OriginKey: origin.Map()}}})
		if err != nil {
			return nil, err
		}

		n := &written[0]
		if ch.LinkTo != "" {
			edge, err := graph.UpsertEdge(ctx, sketchID, graphmodel.Edge{SourceID: ch.LinkTo, TargetID: n.ID, Label: relation(ch.Relation),
				Properties: map[string]any{graphmodel.OriginKey: origin.Map()}})
			if err != nil {
				return n, err
			}

			if edge == nil {
				return n, fmt.Errorf("link_to: %w", errNoNode)
			}
		}

		return n, nil
	case KindLink:
		if ch.Source == "" || ch.Target == "" {
			return nil, errors.New("link needs source and target")
		}

		edge, err := graph.UpsertEdge(ctx, sketchID, graphmodel.Edge{SourceID: ch.Source, TargetID: ch.Target, Label: relation(ch.Label),
			Properties: map[string]any{graphmodel.OriginKey: origin.Map()}})
		if err != nil {
			return nil, err
		}

		if edge == nil {
			return nil, errNoNode
		}

		return nil, nil
	default:
		return nil, fmt.Errorf("unknown kind %q", ch.Kind)
	}
}

// mergeProps lays src over a copy of dst key by key, nested objects
// recursing, as the graph store merges a patch.
func mergeProps(dst, src map[string]any) map[string]any {
	out := maps.Clone(dst)
	if out == nil {
		out = map[string]any{}
	}

	for k, v := range src {
		sub, isMap := v.(map[string]any)
		cur, curIsMap := out[k].(map[string]any)
		if isMap && curIsMap {
			out[k] = mergeProps(cur, sub)

			continue
		}

		out[k] = v
	}

	return out
}
