// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"slices"
	"testing"
)

type listPayload struct {
	Addresses []string `json:"addresses" validate:"required,dive,email" normalize:"normalize"`
}

// A normalize tag on a []string reaches the strings, or the tag on a
// list of addresses is a promise nothing keeps.
func TestNormalizeReachesTheStringsInASlice(t *testing.T) {
	in := listPayload{Addresses: []string{" A@Example.TEST ", "b@example.test"}}
	if err := NormalizeAndValidate(&in); err != nil {
		t.Fatalf("validate: %v", err)
	}

	want := []string{"a@example.test", "b@example.test"}
	if !slices.Equal(in.Addresses, want) {
		t.Errorf("addresses = %q, want %q", in.Addresses, want)
	}
}

// A dive rule names the element, `addresses[1]`. The form has one
// control for the list, so the error is keyed on the list and the
// message keeps the index.
func TestAnElementErrorIsKeyedOnItsList(t *testing.T) {
	in := listPayload{Addresses: []string{"a@example.test", "not-an-address"}}
	err := NormalizeAndValidate(&in)
	if err == nil {
		t.Fatal("a malformed address was accepted")
	}

	fes := Humanize(err)
	if len(fes) != 1 || fes[0].Field != "addresses" {
		t.Fatalf("fields = %+v, want one keyed addresses", fes)
	}

	if fes[0].Message == "" || fes[0].Rule != "email" {
		t.Errorf("message = %q rule = %q", fes[0].Message, fes[0].Rule)
	}
}
