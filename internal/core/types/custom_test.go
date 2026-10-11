// Gosint, Copyright (c) 2026 YouSysAdmin

package types_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/types"
)

// A custom type checks its fields like a built-in one, names itself by
// its primary field, takes a typed line into it, and a built-in name or
// a type with no primary field is refused.
func TestCustomTypesLayerOverTheBuiltInOnes(t *testing.T) {
	reg := types.Builtin()
	t.Cleanup(func() { _ = reg.SetCustom(nil) })

	def := types.CustomType{Name: "badge", Title: "Badge", Category: "person", Icon: "Tag", Color: "#336699",
		Fields: []types.CustomField{
			{Name: "number", Title: "Number", Kind: "string", Primary: true},
			{Name: "issued", Kind: "datetime"},
			{Name: "level", Kind: "int"},
			{Name: "tags", Kind: "list"},
		}}
	if err := reg.SetCustom([]types.CustomType{def}); err != nil {
		t.Fatal(err)
	}

	typ, ok := reg.Get("badge")
	if !ok || typ.Title != "Badge" || len(typ.Fields) != 4 || !typ.Fields[0].Required {
		t.Fatalf("badge = %+v, %v", typ, ok)
	}

	props := map[string]any{"label": "B-17", "level": "3", "tags": []any{"a"}}
	label, err := reg.Build("badge", props)
	if err != nil || label != "B-17" || props["number"] != "B-17" || fmt.Sprint(props["level"]) != "3" {
		t.Errorf("build = %q, %v, props %v", label, err, props)
	}

	if _, err := reg.Build("badge", map[string]any{"number": "x", "level": "three"}); err == nil {
		t.Error("a bad int went through")
	}

	if got := reg.Key("badge", map[string]any{"number": "B-17"}); got != "B-17" {
		t.Errorf("key = %q", got)
	}

	if !func() bool {
		for _, x := range reg.All() {
			if x.Name == "badge" {
				return true
			}
		}

		return false
	}() {
		t.Error("All leaves the custom type out")
	}

	for _, bad := range []types.CustomType{
		{Name: "domain", Title: "x", Category: "global", Fields: def.Fields},
		{Name: "Bad Name", Title: "x", Category: "global", Fields: def.Fields},
		{Name: "noprimary", Title: "x", Category: "global", Fields: []types.CustomField{{Name: "a", Kind: "string"}}},
		{Name: "twice", Title: "x", Category: "global", Fields: []types.CustomField{{Name: "a", Kind: "string", Primary: true}, {Name: "a", Kind: "int"}}},
		{Name: "kind", Title: "x", Category: "global", Fields: []types.CustomField{{Name: "a", Kind: "blob", Primary: true}}},
		{Name: "labelled", Title: "x", Category: "global", Fields: []types.CustomField{{Name: "label", Kind: "string", Primary: true}}},
		{Name: "icon", Title: "x", Category: "global", Icon: "../../x", Fields: def.Fields},
	} {
		if err := reg.CheckCustom(bad); err == nil {
			t.Errorf("%s was accepted", bad.Name)
		}
	}

	// Replaced by an empty set, it is gone and the built-in ones stay.
	if err := reg.SetCustom(nil); err != nil {
		t.Fatal(err)
	}

	if _, ok := reg.Get("badge"); ok {
		t.Error("badge survived the reset")
	}

	if _, ok := reg.Get("domain"); !ok || !strings.HasPrefix("domain", "d") {
		t.Error("the built-in types went with the reset")
	}
}

// A nested field is read through the registry while custom types are
// swapped in, which -race holds to the registry's lock.
func TestANestedFieldReadsUnderTheLock(t *testing.T) {
	reg := types.Builtin()
	t.Cleanup(func() { _ = reg.SetCustom(nil) })

	done := make(chan struct{})
	go func() {
		defer close(done)

		for range 50 {
			_ = reg.SetCustom(nil)
		}
	}()

	for range 50 {
		props := map[string]any{"full_name": "Jane Doe", "current_address": "1 Main St"}
		if _, err := reg.Build("individual", props); err != nil {
			t.Fatal(err)
		}
	}

	<-done
}
