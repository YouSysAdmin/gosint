// Gosint, Copyright (c) 2026 YouSysAdmin

// Package props reads the JSON-shaped property maps of entities and API
// answers.
package props

// Str reads a string property, "" when absent.
func Str(props map[string]any, key string) string {
	s, _ := props[key].(string)

	return s
}

// AsList reads a JSON array, nil when the value is not one.
func AsList(v any) []any {
	l, _ := v.([]any)

	return l
}

// Scalar reports whether a JSON value is a string, number or bool.
func Scalar(v any) bool {
	switch v.(type) {
	case string, float64, bool:
		return true
	default:
		return false
	}
}

// Truncate cuts s to n bytes with an ellipsis.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "..."
}
