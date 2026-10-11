// Gosint, Copyright (c) 2026 YouSysAdmin

package validation_test

import (
	"testing"

	"github.com/yousysadmin/gosint/internal/core/validation"
)

// And the behaviour itself, so the rule above is a fact rather than a
// belief about a dependency. An upgrade that changed either answer
// would leave the whole tree tagged for semantics it no longer has.
func TestOmitzeroSkipsAnEmptyPointerAndOmitemptyDoesNot(t *testing.T) {
	type subject struct {
		Empty *string `validate:"omitempty,email"`
		Zero  *string `validate:"omitzero,email"`
	}
	blank, bad, good := "", "not-an-address", "ops@example.com"

	if err := validation.V().Struct(subject{Empty: &blank}); err == nil {
		t.Error("omitempty now skips an empty pointer - the tree can go back to it")
	}

	if err := validation.V().Struct(subject{Zero: &blank}); err != nil {
		t.Errorf("omitzero rejected an empty pointer: %v", err)
	}

	if err := validation.V().Struct(subject{Zero: &bad}); err == nil {
		t.Error("omitzero skipped a value that is present and wrong")
	}

	if err := validation.V().Struct(subject{Zero: &good}); err != nil {
		t.Errorf("omitzero rejected a good address: %v", err)
	}

	if err := validation.V().Struct(subject{}); err != nil {
		t.Errorf("a nil pointer must pass either way: %v", err)
	}
}
