// Gosint, Copyright (c) 2026 YouSysAdmin

package llm_test

import (
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/llm"
)

// An OpenAI style stream is read token by token, the system prompt and
// the key travel as that API expects, and [DONE] ends it quietly.
func TestStreamsAnOpenAIStyleAnswer(t *testing.T) {
	var got struct {
		Model    string        `json:"model"`
		Messages []llm.Message `json:"messages"`
	}

	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)

			return
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, piece := range []string{"Hel", "lo", " there"} {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"`+piece+`"}}]}`+"\n\n")
		}

		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	// Two system messages, one of them after a turn, as the chat sends
	// its prompt and then the entities: they travel as ONE leading
	// system message, which a Qwen chat template insists on.
	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL + "/v1", Model: "m"}, "k", nil)
	answer, err := c.Complete(t.Context(), []llm.Message{
		{Role: llm.RoleSystem, Content: "be brief"}, {Role: llm.RoleUser, Content: "earlier"},
		{Role: llm.RoleAssistant, Content: "ok"}, {Role: llm.RoleSystem, Content: "entities:\n- domain: a.io"},
		{Role: llm.RoleUser, Content: "hi"},
	})
	if err != nil || answer != "Hello there" {
		t.Fatalf("answer = %q, %v", answer, err)
	}

	if auth != "Bearer k" || got.Model != "m" || len(got.Messages) != 4 || got.Messages[0].Role != "system" ||
		got.Messages[0].Content != "be brief\n\nentities:\n- domain: a.io" || got.Messages[1].Role != "user" {
		t.Errorf("request = auth %q, %+v", auth, got)
	}
}

// Anthropic takes the system prompt beside the turns and streams
// content_block_delta events.
func TestStreamsAnAnthropicAnswer(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != "k" || r.Header.Get("anthropic-version") == "" {
			http.Error(w, "bad request shape", http.StatusBadRequest)

			return
		}

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = io.WriteString(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hi\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIAnthropic, BaseURL: srv.URL, Model: "m"}, "k", nil)
	answer, err := c.Complete(t.Context(), []llm.Message{
		{Role: llm.RoleSystem, Content: "be brief"}, {Role: llm.RoleUser, Content: "hi"},
	})
	if err != nil || answer != "Hi" {
		t.Fatalf("answer = %q, %v", answer, err)
	}

	if got["system"] != "be brief" || len(got["messages"].([]any)) != 1 {
		t.Errorf("request = %v", got)
	}
}

// A refusal carries the API's own message, not just the status.
func TestARefusalNamesTheReason(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key provided"}}`)
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL, Model: "m"}, "bad", nil)
	_, err := c.Complete(t.Context(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "Incorrect API key") {
		t.Errorf("err = %v", err)
	}
}

// A reasoning model that spends its whole output limit thinking sends
// no text at all, which is reported as such rather than as an empty
// answer.
func TestAnAnswerCutWhileThinkingIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"reasoning_content":"hmm"},"finish_reason":null}]}`+"\n\n")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{},"finish_reason":"length"}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL, Model: "m"}, "", nil)
	_, err := c.Complete(t.Context(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	if !errors.Is(err, llm.ErrNoText) || !strings.Contains(err.Error(), "output limit") {
		t.Errorf("err = %v", err)
	}
}

func TestLocalRecognizesTheMachine(t *testing.T) {
	for url, want := range map[string]bool{
		"http://localhost:1234/v1": true, "http://127.0.0.1:11434/v1": true, "http://192.168.1.20:1234/v1": true,
		"https://api.openai.com/v1": false, "nonsense": false,
	} {
		if llm.Local(url) != want {
			t.Errorf("Local(%q) = %v", url, !want)
		}
	}
}

// Reasoning streamed beside the answer, or inline between think tags,
// reaches the caller as thinking and never as answer text.
func TestThinkingIsSeparateFromTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, delta := range []string{
			`{"reasoning_content":"let me see"}`,
			`{"content":"<think>more"}`,
			`{"content":" thought</think>The"}`,
			`{"content":" answer"}`,
		} {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":`+delta+`}]}`+"\n\n")
		}

		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL + "/v1", Model: "m"}, "", nil)

	var text, thinking strings.Builder
	err := c.Stream(t.Context(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, func(p llm.Piece) error {
		text.WriteString(p.Text)
		thinking.WriteString(p.Thinking)

		return nil
	})
	if err != nil || text.String() != "The answer" || thinking.String() != "let me seemore thought" {
		t.Errorf("text %q thinking %q err %v", text.String(), thinking.String(), err)
	}
}

// A tool call streamed in fragments arrives whole, after the text, and
// counts as an answer.
func TestAToolCallArrivesAssembled(t *testing.T) {
	var sentTools int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Tools []any `json:"tools"`
		}
		_ = json.Unmarshal(body, &req)
		sentTools = len(req.Tools)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, delta := range []string{
			`{"content":"Noted."}`,
			`{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"propose_changes","arguments":"{\"changes\":[{\"kind\":"}}]}`,
			`{"tool_calls":[{"index":0,"function":{"arguments":"\"link\"}]}"}}]}`,
		} {
			_, _ = io.WriteString(w, `data: {"choices":[{"delta":`+delta+`}]}`+"\n\n")
		}

		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`+"\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL + "/v1", Model: "m"}, "", nil)
	tool := llm.Tool{Name: "propose_changes", Description: "d", Parameters: map[string]any{"type": "object"}}

	var text string
	var calls []llm.ToolCall
	err := c.StreamTools(t.Context(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, []llm.Tool{tool}, func(p llm.Piece) error {
		text += p.Text
		if p.Call != nil {
			calls = append(calls, *p.Call)
		}

		return nil
	})
	if err != nil || sentTools != 1 || text != "Noted." || len(calls) != 1 || calls[0].ID != "call_1" ||
		calls[0].Arguments != `{"changes":[{"kind":"link"}]}` {
		t.Errorf("text %q calls %+v tools %d err %v", text, calls, sentTools, err)
	}
}

// A server that stops sending mid-answer is given up on, with the
// reason, instead of being waited for.
func TestASilentModelIsGivenUpOn(t *testing.T) {
	saved := llm.IdleTimeout
	llm.IdleTimeout = 300 * time.Millisecond
	t.Cleanup(func() { llm.IdleTimeout = saved })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"choices":[{"delta":{"content":"Hel"}}]}`+"\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: srv.URL + "/v1", Model: "m"}, "", nil)
	start := time.Now()
	_, err := c.Complete(t.Context(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "silent") || time.Since(start) > 5*time.Second {
		t.Errorf("err = %v after %s", err, time.Since(start))
	}
}
