// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"slices"
	"strings"
)

// catalog is every built-in type.
func catalog() []spec {
	return slices.Concat(networkTypes(), identityTypes(), contactTypes(), securityTypes(),
		fileTypes(), financeTypes(), leakTypes(), assetTypes(), findingTypes())
}

// field labels from one field, "" when it is empty.
func field(name string) func(map[string]any) string {
	return func(p map[string]any) string {
		if !truthy(p[name]) {
			return ""
		}

		return pyStr(p[name])
	}
}

// either labels from the first non-empty of two fields.
func either(first, second string) func(map[string]any) string {
	return func(p map[string]any) string {
		if truthy(p[first]) {
			return pyStr(p[first])
		}

		return field(second)(p)
	}
}

// joined labels from the non-empty fields, joined by sep, falling back
// to the primary field when none is set.
func joined(sep, primary string, names ...string) func(map[string]any) string {
	return func(p map[string]any) string {
		var parts []string
		for _, n := range names {
			if truthy(p[n]) {
				parts = append(parts, pyStr(p[n]))
			}
		}

		if len(parts) == 0 {
			return field(primary)(p)
		}

		return strings.Join(parts, sep)
	}
}

// into builds props by putting the trimmed line in one field.
func into(name string) func(string) (map[string]any, error) {
	return func(s string) (map[string]any, error) {
		return map[string]any{name: pyStrip(s)}, nil
	}
}

// masked is the last four characters behind asterisks, how account
// and card numbers are labelled.
func masked(number string) string {
	r := []rune(number)
	if len(r) <= 4 {
		return number
	}

	return "****" + string(r[len(r)-4:])
}
