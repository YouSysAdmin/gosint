// Gosint, Copyright (c) 2026 YouSysAdmin

package llm_test

import (
	"os"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/core/llm"
)

// An opt-in check against a real model server, as GOSINT_TEST_PG_DSN is
// for postgres: GOSINT_TEST_LLM_URL is its base URL, GOSINT_TEST_LLM_MODEL
// the model.
func TestLiveModelAnswers(t *testing.T) {
	base := os.Getenv("GOSINT_TEST_LLM_URL")
	if base == "" {
		t.Skip("GOSINT_TEST_LLM_URL not set")
	}

	c := llm.New(llm.Config{API: llm.APIOpenAI, BaseURL: base, Model: os.Getenv("GOSINT_TEST_LLM_MODEL")}, "", nil)
	pieces := 0
	start := time.Now()
	answer := ""
	err := c.Stream(t.Context(), []llm.Message{
		{Role: llm.RoleSystem, Content: "You write YAML. Answer with the YAML only, no explanations, no markdown fences."},
		{Role: llm.RoleUser, Content: "A YAML with keys name: my-lookup and version: 1"},
	}, func(p llm.Piece) error {
		pieces++
		answer += p.Text

		return nil
	})
	t.Logf("%d pieces in %s, err %v, answer %q", pieces, time.Since(start).Round(time.Millisecond), err, answer)
	if err != nil || answer == "" {
		t.Fatal("no answer")
	}
}
