// Gosint, Copyright (c) 2026 YouSysAdmin

// Package llmmodel is a language model an administrator offers the
// assistant and the template generator.
package llmmodel

import (
	"time"

	"github.com/yousysadmin/gosint/internal/core/llm"
)

// Tool modes.
const (
	ToolsAuto = "auto"
	ToolsOn   = "on"
	ToolsOff  = "off"
)

// Model is one offered model. APIKey never leaves through the API.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// API is the wire style: llm.APIOpenAI, which OpenAI, Mistral,
	// OpenRouter, Ollama and LM Studio speak, or llm.APIAnthropic.
	API string `json:"api"`

	// BaseURL is the API root, the host alone for anthropic.
	BaseURL string `json:"base_url"`

	// Model may stay empty for a local server, which then answers with
	// whatever model it has loaded.
	Model string `json:"model"`

	// Tools says whether the model is offered tools: on, off, or auto,
	// which is on for a remote service and off for a model on this
	// machine, whose constrained decoding of a tool call has stalled.
	Tools string `json:"tools"`

	// KeyName is the secret each account stores its API key under.
	KeyName string `json:"key_name"`

	// APIKey is the server's own key, used when the account has none.
	APIKey string `json:"api_key"`

	// Default is the model of a person who picked none.
	Default bool `json:"-"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

// Local says the model is served on this machine or the local
// network, where it usually wants no key.
func (m *Model) Local() bool { return llm.Local(m.BaseURL) }

// UseTools says whether the model is offered tools: as set, or for
// auto, only a remote service.
func (m *Model) UseTools() bool {
	switch m.Tools {
	case ToolsOn:
		return true
	case ToolsOff:
		return false
	default:
		return !m.Local()
	}
}

// Config is what the client needs to reach it.
func (m *Model) Config() llm.Config {
	return llm.Config{API: m.API, BaseURL: m.BaseURL, Model: m.Model}
}
