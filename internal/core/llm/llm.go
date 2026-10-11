// Gosint, Copyright (c) 2026 YouSysAdmin

// Package llm talks to a chat model over one of two wire styles: the
// OpenAI chat completions API, which OpenAI, Mistral, OpenRouter, Ollama
// and LM Studio all speak, or Anthropic's messages API. Answers are
// streamed token by token.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Wire styles.
const (
	APIOpenAI    = "openai"
	APIAnthropic = "anthropic"
)

// Roles of a message.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"

	// RoleTool is a turn answering a tool call.
	RoleTool = "tool"
)

// maxAnswer bounds one answer, in tokens, on the Anthropic API, which
// requires a limit.
const maxAnswer = 4096

// Config names the model to talk to.
type Config struct {
	// API is APIOpenAI or APIAnthropic.
	API string

	// BaseURL is the API root: https://api.openai.com/v1 or
	// http://localhost:1234/v1 for the OpenAI style, the host alone
	// (https://api.anthropic.com) for Anthropic.
	BaseURL string
	Model   string
}

// Message is one turn of a conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	// ToolCalls are the calls an assistant turn made, echoed back so the
	// model sees what it asked for. ToolCallID names the call a tool turn
	// answers.
	ToolCalls  []ToolCall `json:"tool_calls,omitzero"`
	ToolCallID string     `json:"tool_call_id,omitzero"`
}

// Client streams answers from one model.
type Client struct {
	cfg  Config
	key  string
	http *http.Client
}

// New makes a client. The key may be empty for a model served locally.
func New(cfg Config, key string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute}
	}

	return &Client{cfg: cfg, key: key, http: hc}
}

// Local tells whether a base URL points at this machine, where a model
// server usually wants no key.
func Local(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}

	host := u.Hostname()
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// ErrNoText is an answer that ended without any text, which a
// reasoning model does when its thinking used up the output limit.
var ErrNoText = errors.New("llm: the model answered with no text")

// Piece is one delta of a streamed answer: text of the answer itself,
// thinking, the reasoning a model shows before it answers, drafting,
// a fragment of a tool call as it is spelled (progress, not content),
// or a tool call once complete. A piece carries one.
type Piece struct {
	Text     string
	Thinking string
	Drafting string
	Call     *ToolCall
}

// IdleTimeout ends a stream that delivers nothing for this long. A
// local server stuck in constrained decoding of a tool call sends no
// bytes at all, and without this the chat waited the whole answer
// timeout for it.
var IdleTimeout = 90 * time.Second

// errSilent is the cause of a cancelled stream.
var errSilent = errors.New("llm: the model went silent")

// Tool is a function the model may call. Parameters is a JSON schema.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is one call the model made, its arguments as the JSON text
// the model wrote.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// callDelta is a fragment of a tool call as the stream carries it: the
// start names it, the following ones add to its arguments.
type callDelta struct {
	Index int
	ID    string
	Name  string
	Args  string
}

// thinkTags splits an answer that carries its reasoning inline between
// <think> and </think>, as some servers stream it, into pieces.
type thinkTags struct {
	inside bool
}

func (t *thinkTags) split(text string) []Piece {
	var out []Piece
	for text != "" {
		if t.inside {
			before, after, found := strings.Cut(text, "</think>")
			if before != "" {
				out = append(out, Piece{Thinking: before})
			}

			if !found {
				return out
			}

			t.inside = false
			text = after

			continue
		}

		before, after, found := strings.Cut(text, "<think>")
		if before != "" {
			out = append(out, Piece{Text: before})
		}

		if !found {
			return out
		}

		t.inside = true
		text = after
	}

	return out
}

// Stream sends the conversation and hands each piece of the answer to
// emit as it arrives. An emit error stops the stream.
func (c *Client) Stream(ctx context.Context, msgs []Message, emit func(Piece) error) error {
	return c.StreamTools(ctx, msgs, nil, emit)
}

// StreamTools is Stream with tools the model may call. A call arrives
// as one Piece once its arguments are complete, after the text.
func (c *Client) StreamTools(ctx context.Context, msgs []Message, tools []Tool, emit func(Piece) error) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	// The watchdog: every event pushes it back, silence fires it.
	watchdog := time.AfterFunc(IdleTimeout, func() { cancel(errSilent) })
	defer watchdog.Stop()

	req, err := c.request(ctx, msgs, tools)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("llm: %w", err)
	}

	defer resp.Body.Close() //nolint:errcheck // read-only

	if resp.StatusCode != http.StatusOK {
		return apiError(resp)
	}

	emitted, cut := false, false
	var tags thinkTags
	calls := map[int]*ToolCall{}
	var order []int
	err = readEvents(resp.Body, func(data []byte) error {
		watchdog.Reset(IdleTimeout)
		text, thinking, call, finish, err := c.delta(data)
		if err != nil {
			return err
		}

		cut = cut || finish == "length" || finish == "max_tokens"
		if thinking != "" {
			if err := emit(Piece{Thinking: thinking}); err != nil {
				return err
			}
		}

		if call != nil {
			tc, known := calls[call.Index]
			if !known {
				tc = &ToolCall{}
				calls[call.Index] = tc
				order = append(order, call.Index)
			}

			tc.ID += call.ID
			tc.Name += call.Name
			tc.Arguments += call.Args
			if call.Args != "" {
				if err := emit(Piece{Drafting: call.Args}); err != nil {
					return err
				}
			}
		}

		for _, p := range tags.split(text) {
			emitted = emitted || p.Text != ""
			if err := emit(p); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		if errors.Is(context.Cause(ctx), errSilent) {
			return fmt.Errorf("%w for %s, it may be stuck", errSilent, IdleTimeout)
		}

		return err
	}

	for _, i := range order {
		emitted = true
		if err := emit(Piece{Call: calls[i]}); err != nil {
			return err
		}
	}

	if !emitted {
		if cut {
			return fmt.Errorf("%w: it ran out of output tokens while thinking, raise the model's output limit", ErrNoText)
		}

		return ErrNoText
	}

	return nil
}

// Complete is Stream with the whole answer collected.
func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	var b strings.Builder
	err := c.Stream(ctx, msgs, func(p Piece) error {
		b.WriteString(p.Text)

		return nil
	})

	return b.String(), err
}

func (c *Client) request(ctx context.Context, msgs []Message, tools []Tool) (*http.Request, error) {
	var (
		endpoint string
		body     any
		header   = http.Header{"Content-Type": {"application/json"}, "Accept": {"text/event-stream"}}
	)

	// Every system message is folded into one, ahead of the turns. The
	// caller adds one per concern (the prompt, the entities), and a chat
	// template such as Qwen's refuses a system message anywhere but
	// first. Anthropic takes it beside the turns anyway.
	var system []string
	turns := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == RoleSystem {
			system = append(system, m.Content)
		} else {
			turns = append(turns, m)
		}
	}

	switch c.cfg.API {
	case APIAnthropic:
		endpoint = strings.TrimSuffix(c.cfg.BaseURL, "/") + "/v1/messages"
		header.Set("anthropic-version", "2023-06-01")
		if c.key != "" {
			header.Set("x-api-key", c.key)
		}

		m := map[string]any{"model": c.cfg.Model, "max_tokens": maxAnswer, "stream": true,
			"system": strings.Join(system, "\n\n"), "messages": anthropicTurns(turns)}
		if c.cfg.Model == "" {
			delete(m, "model")
		}

		if len(tools) > 0 {
			var list []map[string]any
			for _, t := range tools {
				list = append(list, map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Parameters})
			}

			m["tools"] = list
		}

		body = m
	default:
		endpoint = strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
		if c.key != "" {
			header.Set("Authorization", "Bearer "+c.key)
		}

		chat := make([]any, 0, len(turns)+1)
		if len(system) > 0 {
			chat = append(chat, map[string]any{"role": RoleSystem, "content": strings.Join(system, "\n\n")})
		}

		for _, m := range turns {
			chat = append(chat, openAITurn(m))
		}

		// No model names whatever a local server has loaded.
		m := map[string]any{"model": c.cfg.Model, "stream": true, "messages": chat}
		if c.cfg.Model == "" {
			delete(m, "model")
		}

		if len(tools) > 0 {
			var list []map[string]any
			for _, t := range tools {
				list = append(list, map[string]any{"type": "function",
					"function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters}})
			}

			m["tools"] = list
			m["tool_choice"] = "auto"
		}

		body = m
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}

	req.Header = header

	return req, nil
}

// delta reads one stream event: the answer text it carries, the
// thinking it carries (reasoning_content as LM Studio and vLLM stream
// it, reasoning as OpenRouter does, thinking_delta on Anthropic), and
// the reason the answer ended when it did. An event that cannot be
// read is skipped, not fatal: a server adds event kinds of its own.
func (c *Client) delta(data []byte) (text, thinking string, call *callDelta, finish string, err error) {
	if c.cfg.API == APIAnthropic {
		var ev struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(data, &ev); err != nil {
			return "", "", nil, "", nil
		}

		switch {
		case ev.Type == "error":
			return "", "", nil, "", errors.New("llm: " + ev.Error.Message)
		case ev.Type == "content_block_start" && ev.ContentBlock.Type == "tool_use":
			return "", "", &callDelta{Index: ev.Index, ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name}, "", nil
		case ev.Type == "content_block_delta" && ev.Delta.Type == "input_json_delta":
			return "", "", &callDelta{Index: ev.Index, Args: ev.Delta.PartialJSON}, "", nil
		case ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta":
			return ev.Delta.Text, "", nil, "", nil
		case ev.Type == "content_block_delta" && ev.Delta.Type == "thinking_delta":
			return "", ev.Delta.Thinking, nil, "", nil
		case ev.Type == "message_delta":
			return "", "", nil, ev.Delta.StopReason, nil
		}

		return "", "", nil, "", nil
	}

	if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
		return "", "", nil, "", nil
	}

	var ev struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
				Reasoning        string `json:"reasoning"`
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &ev); err != nil {
		return "", "", nil, "", nil
	}

	if ev.Error != nil {
		return "", "", nil, "", errors.New("llm: " + ev.Error.Message)
	}

	if len(ev.Choices) == 0 {
		return "", "", nil, "", nil
	}

	d := ev.Choices[0].Delta
	if len(d.ToolCalls) > 0 {
		// One fragment per event in practice, the first is taken.
		tc := d.ToolCalls[0]
		call = &callDelta{Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, Args: tc.Function.Arguments}
	}

	return d.Content, d.ReasoningContent + d.Reasoning, call, ev.Choices[0].FinishReason, nil
}

// readEvents walks a server-sent event stream, handing each data
// payload to fn. A multi-line data field is joined with newlines.
func readEvents(r io.Reader, fn func([]byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)

	var data [][]byte
	flush := func() error {
		if len(data) == 0 {
			return nil
		}

		err := fn(bytes.Join(data, []byte("\n")))
		data = data[:0]

		return err
	}

	for sc.Scan() {
		line := sc.Bytes()
		switch {
		case len(line) == 0:
			if err := flush(); err != nil {
				return err
			}
		case bytes.HasPrefix(line, []byte("data:")):
			data = append(data, bytes.Clone(bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))))
		}
	}

	if err := sc.Err(); err != nil {
		return fmt.Errorf("llm: reading the stream: %w", err)
	}

	return flush()
}

// apiError reads the message of a refused request.
func apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))

	var e struct {
		Error any `json:"error"`
	}

	if json.Unmarshal(body, &e) == nil && e.Error != nil {
		switch x := e.Error.(type) {
		case string:
			return fmt.Errorf("llm: %s: %s", resp.Status, x)
		case map[string]any:
			if msg, ok := x["message"].(string); ok {
				return fmt.Errorf("llm: %s: %s", resp.Status, msg)
			}
		}
	}

	return fmt.Errorf("llm: %s", resp.Status)
}

// openAITurn is one message as the chat completions API spells it: an
// assistant turn carries its tool calls, a tool turn names the call.
func openAITurn(m Message) map[string]any {
	out := map[string]any{"role": m.Role, "content": m.Content}
	if m.ToolCallID != "" {
		out["tool_call_id"] = m.ToolCallID
	}

	if len(m.ToolCalls) > 0 {
		calls := make([]map[string]any, 0, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			calls = append(calls, map[string]any{"id": tc.ID, "type": "function",
				"function": map[string]any{"name": tc.Name, "arguments": tc.Arguments}})
		}

		out["tool_calls"] = calls
	}

	return out
}

// anthropicTurns is the conversation as the messages API spells it: an
// assistant turn with calls is text and tool_use blocks, and the tool
// turns answering it are tool_result blocks in ONE user turn, which the
// API insists on.
func anthropicTurns(turns []Message) []map[string]any {
	var out []map[string]any
	for _, m := range turns {
		switch {
		case m.Role == RoleTool:
			block := map[string]any{"type": "tool_result", "tool_use_id": m.ToolCallID, "content": m.Content}
			if n := len(out); n > 0 && out[n-1]["role"] == RoleUser {
				if blocks, ok := out[n-1]["content"].([]map[string]any); ok {
					out[n-1]["content"] = append(blocks, block)

					continue
				}
			}

			out = append(out, map[string]any{"role": RoleUser, "content": []map[string]any{block}})
		case len(m.ToolCalls) > 0:
			blocks := []map[string]any{}
			if m.Content != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
			}

			for _, tc := range m.ToolCalls {
				var input any = map[string]any{}
				_ = json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, map[string]any{"type": "tool_use", "id": tc.ID, "name": tc.Name, "input": input})
			}

			out = append(out, map[string]any{"role": m.Role, "content": blocks})
		default:
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
		}
	}

	return out
}
