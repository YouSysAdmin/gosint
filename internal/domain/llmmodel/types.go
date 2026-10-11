// Gosint, Copyright (c) 2026 YouSysAdmin

package llmmodel

import "time"

// patchInput is the create and update body, every field optional. The
// API key is never read back, so an absent one keeps the stored one
// and an empty string clears it.
type patchInput struct {
	Name    *string `json:"name"     validate:"omitnil,max=100"`
	API     *string `json:"api"      validate:"omitnil,oneof=openai anthropic"`
	BaseURL *string `json:"base_url" validate:"omitnil,max=400"`
	Model   *string `json:"model"    validate:"omitnil,max=200"`
	Tools   *string `json:"tools"    validate:"omitnil,oneof=auto on off"`
	KeyName *string `json:"key_name" validate:"omitnil,keyname"`
	APIKey  *string `json:"api_key"  validate:"omitnil,max=1000"`
	Default *bool   `json:"default"`
}

// AdminView is one model as the panel sees it: has_api_key, never the
// key.
type AdminView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	API       string    `json:"api"`
	BaseURL   string    `json:"base_url"`
	Model     string    `json:"model"`
	Tools     string    `json:"tools"`
	KeyName   string    `json:"key_name"`
	HasAPIKey bool      `json:"has_api_key"`
	Default   bool      `json:"default"`
	Local     bool      `json:"local"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PickView is one model as a person picking it sees it. NeedsKey says
// the person must store KeyName in Secrets to use it.
type PickView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	Default  bool   `json:"default"`
	KeyName  string `json:"key_name"`
	NeedsKey bool   `json:"needs_key"`
}

type adminListResponse struct {
	Models []AdminView `json:"models"`
}

type adminOneResponse struct {
	Model AdminView `json:"model"`
}

type pickListResponse struct {
	Models []PickView `json:"models"`
}
