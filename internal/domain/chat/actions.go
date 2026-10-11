// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/webtext"
)

// The model may LOOK THINGS UP before it answers: read a web page, see
// what hangs off an entity, find an entity by name. It asks, the server
// does it, the result goes back as the next turn, and the model goes
// on. Reading only: writing is a proposal the person applies.

// The kinds of lookup.
const (
	ActFetchURL   = "fetch_url"
	ActNeighbours = "neighbours"
	ActFind       = "find_entity"
)

// actionsFence opens the block a model without tools asks in.
const actionsFence = "```gosint-actions"

// The bounds of a lookup round.
const (
	maxSteps      = 6
	maxActions    = 5
	fetchTimeout  = 25 * time.Second
	fetchMaxBytes = 2 << 20
	resultChars   = 12000
	findLimit     = 20
)

// lookupTool is the reading tool.
var lookupTool = llm.Tool{
	Name: "lookup",
	Description: "Look something up before answering: fetch_url reads a public web page or API URL and returns its text, " +
		"neighbours lists the entities linked to an entity, find_entity finds entities of the sketch by name. " +
		"Results come back as the next message.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"kind":    map[string]any{"type": "string", "enum": []string{ActFetchURL, ActNeighbours, ActFind}},
			"url":     map[string]any{"type": "string", "description": "fetch_url: the http(s) URL"},
			"node_id": map[string]any{"type": "string", "description": "neighbours: the entity id"},
			"query":   map[string]any{"type": "string", "description": "find_entity: part of a label"},
		},
		"required": []string{"kind"},
	},
}

// The two ways to ask, told to the model by what it has.
const (
	lookupShape = "Lookups: fetch_url (url) reads a public web page or API and returns its text, neighbours (node_id) " +
		"lists what is linked to an entity, find_entity (query) finds entities by name. Facts about a specific " +
		"person, account, domain or site must come from a lookup, not from memory: when an entity names a URL " +
		"(profile_url, url), read it before filling its properties, and say which source each fact came from. "

	lookupToolPrompt = "Use the lookup tool to read sources before answering. " + lookupShape

	lookupBlockPrompt = "To look something up, your whole message is one fenced block that starts with " + actionsFence +
		" and holds {\"actions\": [{\"kind\": \"fetch_url\", \"url\": \"...\"}]}. The results arrive in the next " +
		"message and you answer from them. " + lookupShape
)

// Action is one lookup or run the model asked for.
type Action struct {
	Kind   string `json:"kind"`
	URL    string `json:"url,omitzero"`
	NodeID string `json:"node_id,omitzero"`
	Query  string `json:"query,omitzero"`

	// A run (run.go): one transform with its params, or a flow of steps
	// from an input type, on node_ids or the selection.
	Transform string         `json:"transform,omitzero"`
	Params    map[string]any `json:"params,omitzero"`
	NodeIDs   []string       `json:"node_ids,omitzero"`
	Input     string         `json:"input,omitzero"`
	Steps     []runStep      `json:"steps,omitzero"`
	Reason    string         `json:"reason,omitzero"`

	// callID is the tool call that asked, "" for a block.
	callID string
}

// label is how the action is shown while it runs.
func (a Action) label() string {
	switch a.Kind {
	case ActFetchURL:
		return "Reading " + a.URL
	case ActNeighbours:
		return "Looking at the neighbours of " + a.NodeID
	case ActRunTransform, ActRunFlow:
		return a.runLabel()
	default:
		return "Finding " + strings.TrimSpace(a.Query)
	}
}

type actionList struct {
	Actions []Action `json:"actions"`
}

// actionsOf reads the lookups out of the model's calls and its text,
// returning the text without the block.
func actionsOf(text string, calls []llm.ToolCall) (string, []Action) {
	var out []Action
	for _, c := range calls {
		if c.Name != lookupTool.Name && c.Name != runTool.Name {
			continue
		}

		var a Action
		if err := json.Unmarshal([]byte(c.Arguments), &a); err == nil && a.Kind != "" {
			a.callID = c.ID
			out = append(out, a)
		}
	}

	text = takeBlocks(text, func(body []byte) bool {
		var list actionList
		if err := json.Unmarshal(body, &list); err != nil || list.Actions == nil {
			return false
		}

		out = append(out, list.Actions...)

		return true
	})

	if len(out) > maxActions {
		out = out[:maxActions]
	}

	return text, out
}

// act performs one lookup and renders its result for the model. An
// error is a result too, so the model can say what it could not read.
func (h Handler) act(ctx context.Context, sketchID string, a Action) string {
	result, err := h.lookup(ctx, sketchID, a)
	if err != nil {
		return "error: " + err.Error()
	}

	if len(result) > resultChars {
		result = result[:resultChars] + "\n... (cut)"
	}

	return result
}

func (h Handler) lookup(ctx context.Context, sketchID string, a Action) (string, error) {
	switch a.Kind {
	case ActFetchURL:
		return h.fetch(ctx, a.URL)
	case ActNeighbours:
		id, err := newResolverFrom(ctx, h, sketchID).resolve(a.NodeID)
		if err != nil {
			return "", err
		}

		center, edges, err := h.Runtime.Store.Graph.Neighbors(ctx, sketchID, id)
		if err != nil {
			return "", err
		}

		if len(center) == 0 {
			return "", errNoNode
		}

		var b strings.Builder
		for _, n := range center {
			b.WriteString(describeNode(n))
		}

		for _, e := range edges {
			fmt.Fprintf(&b, "- edge [%s] %s -> [%s]\n", e.SourceID, e.Label, e.TargetID)
		}

		return b.String(), nil
	case ActFind:
		q := strings.ToLower(strings.TrimSpace(a.Query))
		if q == "" {
			return "", errors.New("find_entity needs a query")
		}

		nodes, _, err := h.Runtime.Store.Graph.Graph(ctx, sketchID)
		if err != nil {
			return "", err
		}

		var b strings.Builder
		found := 0
		for _, n := range nodes {
			if strings.Contains(strings.ToLower(n.Label), q) || strings.Contains(strings.ToLower(n.Type), q) {
				b.WriteString(describeNode(n))
				if found++; found >= findLimit {
					break
				}
			}
		}

		if found == 0 {
			return "no entity matches " + a.Query, nil
		}

		return b.String(), nil
	default:
		return "", fmt.Errorf("unknown lookup %q", a.Kind)
	}
}

// fetch reads a URL through the client that refuses internal addresses,
// so the model cannot be talked into reading what only the server can
// reach. HTML becomes text, anything else is returned as it is.
func (h Handler) fetch(ctx context.Context, raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("fetch_url needs an http(s) URL")
	}

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "text/html,application/json,text/plain,*/*")
	resp, err := h.Runtime.TargetHTTP.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	body, err := io.ReadAll(io.LimitReader(resp.Body, fetchMaxBytes))
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("%s answered %s", u.Host, resp.Status)
	}

	if strings.Contains(resp.Header.Get("Content-Type"), "html") {
		p := webtext.Parse(body)
		var b strings.Builder
		if p.Title != "" {
			b.WriteString("Title: " + p.Title + "\n")
		}

		if p.Description != "" {
			b.WriteString("Description: " + p.Description + "\n")
		}

		b.WriteString(p.Text)

		return b.String(), nil
	}

	return string(body), nil
}

// newResolverFrom builds a resolver over the sketch, for a lookup that
// may name an entity by label.
func newResolverFrom(ctx context.Context, h Handler, sketchID string) *resolver {
	nodes, _, err := h.Runtime.Store.Graph.Graph(ctx, sketchID)
	if err != nil {
		return newResolver(nil)
	}

	return newResolver(nodes)
}

// resultsTurns are the lookups' results as turns: tool turns answering
// each call, and one user turn for the block, which has no call ids.
func resultsTurns(actions []Action, results []string) []llm.Message {
	var out []llm.Message
	var block strings.Builder
	for i, a := range actions {
		if a.callID != "" {
			out = append(out, llm.Message{Role: llm.RoleTool, ToolCallID: a.callID, Content: results[i]})

			continue
		}

		fmt.Fprintf(&block, "Result of %s %s:\n%s\n\n", a.Kind, strings.TrimSpace(a.URL+a.NodeID+a.Query+a.Transform), results[i])
	}

	if block.Len() > 0 {
		out = append(out, llm.Message{Role: llm.RoleUser, Content: "Results, continue your answer:\n\n" + block.String()})
	}

	return out
}
