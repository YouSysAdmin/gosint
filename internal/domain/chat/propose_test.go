// Gosint, Copyright (c) 2026 YouSysAdmin

package chat

import (
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/llm"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// A proposal arrives as a tool call, or as the block a model without
// tools writes, and either way leaves the answer as the block.
func TestChangesComeFromTheToolOrTheBlock(t *testing.T) {
	text, changes := changesOf("Looks like a CDN.", []llm.ToolCall{{Name: "propose_changes",
		Arguments: `{"changes":[{"kind":"set_properties","node_id":"n1","properties":{"isp":"Cloudflare"},"reason":"ASN"}]}`}})
	if text != "Looks like a CDN." || len(changes) != 1 || changes[0].Kind != KindSetProperties || changes[0].Properties["isp"] != "Cloudflare" {
		t.Errorf("from the tool: %q %+v", text, changes)
	}

	inline := "The registrant is Acme.\n\n```gosint-changes\n{\"changes\":[{\"kind\":\"add_entity\",\"type\":\"organization\"," +
		"\"properties\":{\"name\":\"Acme\"},\"link_to\":\"n1\",\"relation\":\"REGISTERED_BY\"}]}\n```\nAnything else?"
	text, changes = changesOf(inline, nil)
	if text != "The registrant is Acme.\n\nAnything else?" || len(changes) != 1 || changes[0].Type != "organization" {
		t.Errorf("from the block: %q %+v", text, changes)
	}

	stored := withBlock(text, changes)
	if !strings.HasPrefix(stored, "The registrant is Acme.") || !strings.Contains(stored, blockFence) {
		t.Errorf("stored = %q", stored)
	}

	// Stored and read again: the same changes, the same text.
	again, back := changesOf(stored, nil)
	if again != text || len(back) != 1 || back[0].LinkTo != "n1" {
		t.Errorf("round trip = %q %+v", again, back)
	}

	// A block that does not parse stays in the text, for the person.
	kept, none := changesOf("x\n```gosint-changes\nnot json\n```", nil)
	if none != nil || !strings.Contains(kept, "not json") {
		t.Errorf("bad block = %q %+v", kept, none)
	}
}

// A model that wrote the label it read, with a stray comma, still names
// the entity.
func TestReferencesResolveByIdOrLabel(t *testing.T) {
	r := newResolver([]graphmodel.Node{{ID: "01a1-id", Label: "jane@GitHub"}})
	for ref, want := range map[string]string{"01a1-id": "01a1-id", "jane@GitHub, ": "01a1-id", "JANE@github": "01a1-id", "nobody": "nobody", "": ""} {
		if got, err := r.resolve(ref); got != want || err != nil {
			t.Errorf("resolve(%q) = %q, %v, want %q", ref, got, err, want)
		}
	}

	// A label two entities share names neither: the model is told to
	// give the id instead of the last one winning.
	r.add(graphmodel.Node{ID: "02b2-id", Type: "individual", Label: "Jane@github"})
	if got, err := r.resolve("jane@github"); got != "" || err == nil || !strings.Contains(err.Error(), "02b2-id") {
		t.Errorf("an ambiguous label = %q, %v", got, err)
	}

	r.add(graphmodel.Node{ID: "02b2-id", Type: "individual", Label: "Jane@github"})
	if _, err := r.resolve("01a1-id"); err != nil {
		t.Errorf("an id = %v", err)
	}
}

// A lookup arrives as a tool call or as the block, and the block leaves
// the text.
func TestActionsComeFromTheToolOrTheBlock(t *testing.T) {
	text, acts := actionsOf("Let me check.\n```gosint-actions\n{\"actions\":[{\"kind\":\"fetch_url\",\"url\":\"https://x.test/p\"}]}\n```",
		[]llm.ToolCall{{ID: "c1", Name: "lookup", Arguments: `{"kind":"find_entity","query":"acme"}`}})
	if text != "Let me check." || len(acts) != 2 || acts[0].Kind != ActFind || acts[0].callID != "c1" || acts[1].URL != "https://x.test/p" {
		t.Errorf("text %q actions %+v", text, acts)
	}

	turns := resultsTurns(acts, []string{"one", "two"})
	if len(turns) != 2 || turns[0].Role != llm.RoleTool || turns[0].ToolCallID != "c1" || turns[1].Role != llm.RoleUser ||
		!strings.Contains(turns[1].Content, "two") {
		t.Errorf("turns = %+v", turns)
	}
}

// A model writes what it read, as text: counts and flags are typed as
// the field says before the type sees them.
func TestValuesAreTypedAsTheFieldSays(t *testing.T) {
	got := coerce("socialaccount", map[string]any{"followers_count": "1,234", "posts_count": "3.4k", "display_name": "Jane", "is_verified": "true"})
	if got["followers_count"] != int64(1234) || got["posts_count"] != int64(3400) || got["display_name"] != "Jane" {
		t.Errorf("coerced = %#v", got)
	}

	// Any fenced JSON with the key is a proposal, the fence name is not required.
	text, changes := changesOf("Done.\n```json\n{\"changes\":[{\"kind\":\"link\",\"source\":\"a\",\"target\":\"b\"}]}\n```\n", nil)
	if text != "Done." || len(changes) != 1 || changes[0].Kind != KindLink {
		t.Errorf("json fence: %q %+v", text, changes)
	}

	// A code block that is not a proposal stays.
	kept, none := changesOf("Run this:\n```sh\nls -la\n```", nil)
	if none != nil || !strings.Contains(kept, "ls -la") {
		t.Errorf("code kept = %q %+v", kept, none)
	}
}

// A model repeating one line is stopped.
func TestALoopingModelIsStopped(t *testing.T) {
	var g loopGuard
	for i := range loopRepeats - 1 {
		if g.add("*Wait, I'll output the lookup block.*\n") {
			t.Fatalf("tripped after %d repeats", i+1)
		}
	}

	if !g.add("*Wait, I'll output the lookup block.*\n") {
		t.Error("did not trip at the limit")
	}

	var fine loopGuard
	for i := range 100 {
		if fine.add("ok\n") || fine.add("a different line every time "+strings.Repeat("x", i)+"\n") {
			t.Error("tripped on short or varied lines")
		}
	}
}

// A secret field is named to the model, never shown: the model may be a
// remote service.
func TestSecretsAreNamedNotShown(t *testing.T) {
	line := describeNode(graphmodel.Node{ID: "n1", Type: "credential", Label: "root@ssh",
		Properties: map[string]any{"username": "root", "service": "ssh", "password": "hunter2", "value": "AKIAIOSFODNN7EXAMPLE"}}) //nolint:gosec // AWS's documented example key
	if strings.Contains(line, "hunter2") || strings.Contains(line, "AKIA") || !strings.Contains(line, "password: (secret, not shown)") {
		t.Errorf("line = %q", line)
	}
}
