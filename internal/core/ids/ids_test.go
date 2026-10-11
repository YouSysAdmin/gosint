// Gosint, Copyright (c) 2026 YouSysAdmin

package ids_test

import (
	"slices"
	"testing"
	"uuid"

	"github.com/yousysadmin/gosint/internal/core/ids"
)

func TestNewIsVersion7AndOrdered(t *testing.T) {
	const n = 5000
	out := make([]string, 0, n)
	for range n {
		s := ids.New()
		u, err := uuid.Parse(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}

		if v := u[6] >> 4; v != 7 {
			t.Fatalf("%s is version %d, want 7", s, v)
		}

		out = append(out, s)
	}

	if !slices.IsSorted(out) {
		t.Error("ids came back out of creation order")
	}

	if len(slices.Compact(slices.Clone(out))) != n {
		t.Error("two ids collided")
	}
}

func TestValid(t *testing.T) {
	if !ids.Valid(ids.New()) {
		t.Error("a minted id is not valid")
	}

	for _, s := range []string{"", "banana", "0000"} {
		if ids.Valid(s) {
			t.Errorf("ids.Valid(%q) = true", s)
		}
	}
}
