// Gosint, Copyright (c) 2026 YouSysAdmin

// Package chat is the assistant: conversations about a sketch, with the
// selected entities as context, answered by the configured model.
package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/llm"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/llmmodel"
	"github.com/yousysadmin/gosint/internal/domain/vaultkey"
	chatmodel "github.com/yousysadmin/gosint/internal/models/chat"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	lmmodel "github.com/yousysadmin/gosint/internal/models/llmmodel"
)

// ErrNoKey is a model behind a key the account has not stored.
var ErrNoKey = errors.New("no API key for the model")

const (
	// historyTurns is how many earlier turns the model is shown.
	historyTurns = 10

	// contextChars bounds the rendered entities, so a large selection
	// does not crowd the question out.
	contextChars = 8000

	// answerTimeout bounds one streamed answer.
	answerTimeout = 10 * time.Minute

	// titleRunes is how much of the first question names the chat.
	titleRunes = 60
)

// systemPrompt sets the role and says how entities arrive.
const systemPrompt = "You are a CTI/OSINT investigator and you are trying to investigate on a " +
	"variety of real life cases. Use your knowledge and analytics capabilities " +
	"to analyse the context and answer the question the best you can. If you " +
	"need to reference some items (an IP, a domain or something particular) " +
	"please use the code brackets, like : `12.23.34.54` to reference it. " +
	"When entities from the sketch's graph are given as context, each line is " +
	"one entity: its id in brackets, its type, its label and its known properties. "

// Handler serves chats.
type Handler struct {
	Runtime *env.Runtime
}

// ClientFor makes the model client for an account: the model it picked
// (or the default one), behind the account's secret, or the server's
// key, or no key for a model served on this machine.
func ClientFor(ctx context.Context, rt *env.Runtime, ownerID, modelID string) (*llm.Client, *lmmodel.Model, error) {
	m, err := llmmodel.Resolve(ctx, rt, modelID)
	if err != nil {
		return nil, nil, err
	}

	key, ok, err := vaultkey.Secret(ctx, rt, ownerID, m.KeyName)
	if err != nil {
		return nil, nil, err
	}

	if !ok {
		key = m.APIKey
	}

	if key == "" && !m.Local() {
		return nil, nil, fmt.Errorf("%w: store %s in Secrets to use %s", ErrNoKey, m.KeyName, m.Name)
	}

	return llm.New(m.Config(), key, nil), m, nil
}

// Refused says err is the person's to fix, a 400: no model set up, or
// no key for it.
func Refused(err error) bool {
	return errors.Is(err, ErrNoKey) || errors.Is(err, llmmodel.ErrNoModel)
}

// List answers the account's chats about the sketch.
func (h Handler) List(c fiber.Ctx) error {
	chats, err := h.Runtime.Store.Chat.List(c, access.Sketch(c).ID, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, chatsResponse{Chats: slices.Clip(append([]chatmodel.Chat{}, chats...))})
}

// Create starts a chat.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[createInput](c)
	if !ok {
		return resp
	}

	now := time.Now().UTC()
	ch := &chatmodel.Chat{ID: ids.New(), SketchID: access.Sketch(c).ID, OwnerID: access.User(c).ID, Title: in.Title,
		CreatedAt: now, UpdatedAt: now}
	if err := h.Runtime.Store.Chat.Create(c, ch); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, chatResponse{Chat: *ch})
}

// Delete removes a chat and its turns.
func (h Handler) Delete(c fiber.Ctx) error {
	gone, err := h.Runtime.Store.Chat.Delete(c, access.Sketch(c).ID, access.User(c).ID, c.Params("chat_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !gone {
		return response.NotFound(c, "chat not found")
	}

	return response.NoContent(c)
}

// Messages lists the turns of a chat.
func (h Handler) Messages(c fiber.Ctx) error {
	ch, err := h.Runtime.Store.Chat.Get(c, access.Sketch(c).ID, access.User(c).ID, c.Params("chat_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if ch == nil {
		return response.NotFound(c, "chat not found")
	}

	msgs, err := h.Runtime.Store.Chat.Messages(c, ch.ID, 500)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, messagesResponse{Messages: slices.Clip(append([]chatmodel.Message{}, msgs...))})
}

// Send asks the model and streams its answer as server-sent events:
// "token" pieces, then "done" with the stored turn, or "error".
func (h Handler) Send(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[sendInput](c)
	if !ok {
		return resp
	}

	sketch, user := access.Sketch(c), access.User(c)
	ch, err := h.Runtime.Store.Chat.Get(c, sketch.ID, user.ID, c.Params("chat_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if ch == nil {
		return response.NotFound(c, "chat not found")
	}

	client, model, err := ClientFor(c, h.Runtime, user.ID, in.ModelID)
	if Refused(err) {
		return response.BadRequest(c, err.Error())
	}

	if err != nil {
		return response.Internal(c, err)
	}

	history, err := h.Runtime.Store.Chat.Messages(c, ch.ID, historyTurns)
	if err != nil {
		return response.Internal(c, err)
	}

	contextIDs := in.NodeIDs
	if in.WithChildren && len(contextIDs) > 0 {
		if contextIDs, err = h.withChildren(c, sketch.ID, contextIDs); err != nil {
			return response.Internal(c, err)
		}
	}

	entities, fields, err := h.describe(c, sketch.ID, contextIDs)
	if err != nil {
		return response.Internal(c, err)
	}

	now := time.Now().UTC()
	question := &chatmodel.Message{ID: ids.New(), ChatID: ch.ID, Role: llm.RoleUser, Content: in.Prompt,
		Context: contextIDs, CreatedAt: now}
	if err := h.Runtime.Store.Chat.AddMessage(c, question); err != nil {
		return response.Internal(c, err)
	}

	title := ""
	if ch.Title == "" {
		title = firstLine(in.Prompt, titleRunes)
	}

	if err := h.Runtime.Store.Chat.Touch(c, ch.ID, title, now); err != nil {
		return response.Internal(c, err)
	}

	// The model runs transforms only for a person who could themselves.
	canRun := access.Investigation(c).Role.Can(invmodel.ActionUpdate)
	useTools := model.UseTools()
	runs := ""
	if canRun {
		runs = h.catalog(c, sketch.ID, contextIDs)
	}

	msgs := conversation(sketch.Title, history, entities, fields, runs, in.Prompt, useTools, canRun)

	var tools []llm.Tool
	if useTools {
		tools = []llm.Tool{lookupTool, proposeTool}
		if canRun {
			tools = append(tools, runTool)
		}
	}

	store, log := h.Runtime.Store.Chat, h.Runtime.Log

	c.Set(fiber.HeaderContentType, "text/event-stream")
	c.Set(fiber.HeaderCacheControl, "no-cache")
	c.Set("X-Accel-Buffering", "no")

	return c.SendStreamWriter(func(w *bufio.Writer) {
		// Runs after the handler returned, outside the recover middleware
		// and the request's context.
		defer func() {
			if r := recover(); r != nil {
				log.Error("chat stream panicked", "chat", ch.ID, "reason", fmt.Sprint(r))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
		defer cancel()

		// The thinking goes to the reader as it happens, so a long pause
		// shows as work, but only the answer is kept. A step that asks
		// for lookups or runs gets their results as the next turns and
		// runs again, up to maxSteps, then the last step's text is the
		// answer. One run per step: a run takes a while and writes.
		var (
			texts   []string
			changes []Change
			err     error
		)

		for step := range maxSteps {
			var (
				answer strings.Builder
				calls  []llm.ToolCall
			)

			var guard loopGuard
			err = client.StreamTools(ctx, msgs, tools, func(p llm.Piece) error {
				switch {
				case p.Thinking != "":
					if guard.add(p.Thinking) {
						return errLooping
					}

					return frame(w, "thinking", tokenEvent{Text: p.Thinking})
				case p.Drafting != "":
					// Progress only, the call itself arrives whole.
					return frame(w, "drafting", tokenEvent{Text: p.Drafting})
				case p.Call != nil:
					calls = append(calls, *p.Call)

					return nil
				}

				answer.WriteString(p.Text)
				if guard.add(p.Text) {
					return errLooping
				}

				return frame(w, "token", tokenEvent{Text: p.Text})
			})
			if err != nil {
				break
			}

			text, actions := actionsOf(answer.String(), calls)
			text, proposed := changesOf(text, calls)
			changes = append(changes, proposed...)
			if text != "" {
				texts = append(texts, text)
			}

			if len(actions) == 0 || step == maxSteps-1 {
				break
			}

			results := make([]string, len(actions))
			ran := false
			for i, a := range actions {
				if !a.isRun() {
					_ = frame(w, "action", tokenEvent{Text: a.label()})
					results[i] = h.act(ctx, sketch.ID, a)

					continue
				}

				switch {
				case !canRun:
					results[i] = "error: " + readOnlyPrompt
				case ran:
					results[i] = "error: one run per message, ask for this one in your next message"
				default:
					ran = true
					if len(a.NodeIDs) == 0 {
						a.NodeIDs = contextIDs
					}

					_ = frame(w, "action", tokenEvent{Text: a.label()})
					progress := func(text string) { _ = frame(w, "progress", tokenEvent{Text: text}) }
					result, confirm := h.run(ctx, sketch.ID, user.ID, a, progress)
					results[i] = result
					if confirm != nil {
						changes = append(changes, *confirm)
					}
				}
			}

			_ = frame(w, "token", tokenEvent{Text: "\n\n"})
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: text, ToolCalls: calls})
			msgs = append(msgs, resultsTurns(actions, results)...)
		}

		// A proposal, from the tool or from the block the model wrote,
		// is kept in the answer as the block, which the console draws
		// as cards to apply.
		content := withBlock(strings.Join(texts, "\n\n"), changes)
		if content != "" || err == nil {
			reply := &chatmodel.Message{ID: ids.New(), ChatID: ch.ID, Role: llm.RoleAssistant, Content: content,
				CreatedAt: time.Now().UTC()}
			if saveErr := store.AddMessage(ctx, reply); saveErr != nil {
				log.Error("chat answer not stored", "chat", ch.ID, "err", saveErr)
			} else if err == nil {
				_ = frame(w, "done", doneEvent{Message: *reply})
			}
		}

		if err != nil {
			// The detail names the provider and its account, so it stays
			// in the log.
			log.Warn("chat answer failed", "chat", ch.ID, "err", err)
			_ = frame(w, "error", errorEvent{Error: "the model did not answer"})
		}
	})
}

// frame writes one event and flushes it.
func frame(w *bufio.Writer, event string, data any) error {
	body, err := response.Marshal(data)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, body); err != nil {
		return err
	}

	return w.Flush()
}

// maxChildren bounds how far a selection grows through its children,
// the context is capped by characters anyway.
const maxChildren = 300

// withChildren adds to ids every node reached by following edges from
// a selected one onward, source to target, breadth first.
func (h Handler) withChildren(ctx context.Context, sketchID string, ids []string) ([]string, error) {
	_, edges, err := h.Runtime.Store.Graph.Graph(ctx, sketchID)
	if err != nil {
		return nil, err
	}

	children := map[string][]string{}
	for _, e := range edges {
		children[e.SourceID] = append(children[e.SourceID], e.TargetID)
	}

	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}

	for i := 0; i < len(out) && len(out) < maxChildren; i++ {
		for _, child := range children[out[i]] {
			if !seen[child] {
				seen[child] = true
				out = append(out, child)
			}
		}
	}

	return out, nil
}

// catalog is the transforms the model may run from the selection.
func (h Handler) catalog(ctx context.Context, sketchID string, nodeIDs []string) string {
	nodes, err := h.Runtime.Store.Graph.NodesIn(ctx, sketchID, nodeIDs)
	if err != nil {
		nodes = nil
	}

	return transformCatalog(h.Runtime.Transforms, nodes)
}

// describe renders the selected entities for the model, one per line,
// and the fields of their types, so a proposal names real properties.
func (h Handler) describe(ctx context.Context, sketchID string, nodeIDs []string) (entities, fields string, err error) {
	nodes, err := h.Runtime.Store.Graph.NodesIn(ctx, sketchID, nodeIDs)
	if err != nil {
		return "", "", err
	}

	var b strings.Builder
	for _, n := range nodes {
		line := describeNode(n)
		if b.Len()+len(line) > contextChars {
			b.WriteString("- (more entities left out)\n")

			break
		}

		b.WriteString(line)
	}

	return b.String(), catalog(nodes), nil
}

// describeNode is one entity as "- type: label (key: value, ...)".
func describeNode(n graphmodel.Node) string {
	// A secret field (a password, a found key) is named, never shown:
	// the model may be a remote service.
	secret := types.Builtin().SecretFields(n.Type)

	var parts []string
	for _, k := range slices.Sorted(maps.Keys(n.Properties)) {
		v := n.Properties[k]
		if v == nil || v == "" || v == false {
			continue
		}

		if slices.Contains(secret, k) {
			parts = append(parts, k+": (secret, not shown)")

			continue
		}

		if _, isMap := v.(map[string]any); isMap {
			continue
		}

		s := fmt.Sprint(v)
		if len(s) > 200 {
			s = s[:200] + "..."
		}

		parts = append(parts, k+": "+s)
	}

	line := "- [" + n.ID + "] " + n.Type + ": " + n.Label
	if len(parts) > 0 {
		line += " (" + strings.Join(parts, ", ") + ")"
	}

	return line + "\n"
}

// conversation is what the model is shown: the system prompt, the
// earlier turns, the selected entities and the question. runs is the
// catalog of transforms the model may run, for a person who can.
func conversation(sketchTitle string, history []chatmodel.Message, entities, fields, runs, prompt string, tools, canRun bool) []llm.Message {
	how := lookupBlockPrompt + " " + proposeBlockPrompt
	if tools {
		how = lookupToolPrompt + " " + proposeToolPrompt
	}

	switch {
	case !canRun:
		how += " " + readOnlyPrompt
	case tools:
		how += " " + runToolPrompt
	default:
		how += " " + runBlockPrompt
	}

	if canRun && runs != "" {
		how += "\n\nTransforms you may run (name: input -> output):\n" + runs
	}

	msgs := []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt + how}}
	for _, m := range history {
		if m.Content != "" {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
	}

	if entities != "" {
		msgs = append(msgs, llm.Message{Role: llm.RoleSystem,
			Content: "Entities selected in the sketch \"" + sketchTitle + "\":\n" + entities +
				"\nProperties each of these types can hold:\n" + fields})
	}

	return append(msgs, llm.Message{Role: llm.RoleUser, Content: prompt})
}

// firstLine is the start of s, for a title.
func firstLine(s string, runes int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if utf8.RuneCountInString(s) <= runes {
		return s
	}

	r := []rune(s)

	return strings.TrimSpace(string(r[:runes])) + "..."
}

// errLooping is a model repeating itself, which a small model does
// when an instruction confuses it, until its token limit.
var errLooping = errors.New("the model got stuck repeating itself")

// loopGuard watches the lines a model writes and trips when one line
// comes back loopRepeats times.
type loopGuard struct {
	line  strings.Builder
	seen  map[string]int
	trips bool
}

const loopRepeats = 12

// add takes a piece of the answer and reports whether the model loops.
func (g *loopGuard) add(piece string) bool {
	if g.seen == nil {
		g.seen = map[string]int{}
	}

	for _, r := range piece {
		if r != '\n' {
			g.line.WriteRune(r)

			continue
		}

		line := strings.TrimSpace(g.line.String())
		g.line.Reset()
		if len(line) < 12 {
			continue
		}

		g.seen[line]++
		if g.seen[line] >= loopRepeats {
			g.trips = true
		}
	}

	return g.trips
}
