// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"encoding/json/v2"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// pyStrip trims like Python's str.strip(), which also treats the
// \x1c-\x1f separators as whitespace.
func pyStrip(s string) string {
	return strings.TrimFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
	})
}

// str is props[key] when it is a string, "" otherwise.
func str(props map[string]any, key string) string {
	s, _ := props[key].(string)

	return s
}

// Truthy reports whether v holds a value: not nil, "", 0, false or an
// empty list.
func Truthy(v any) bool { return truthy(v) }

// truthy is Python truthiness over JSON values.
func truthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return t != ""
	case bool:
		return t
	case float64:
		return t != 0
	case int64:
		return t != 0
	case int:
		return t != 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	default:
		return true
	}
}

// pyStr formats a value the way Python's str() would in a label.
func pyStr(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case bool:
		if t {
			return "True"
		}

		return "False"
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	case float64:
		return pyFloat(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return ""
		}

		return string(b)
	}
}

// pyFloat is Python's repr of a float: shortest round trip, ".0" on an
// integral value, exponent form outside 1e-4..1e16.
func pyFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}

	if a := math.Abs(f); a != 0 && (a < 1e-4 || a >= 1e16) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}

	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}

	return s
}

// firstRunes is s cut to n code points.
func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n])
}

// toInt coerces like pydantic's lax int: an int, an integral float, a
// numeric string or a bool.
func toInt(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		if t != math.Trunc(t) || math.IsInf(t, 0) || t > math.MaxInt64 || t < math.MinInt64 {
			return 0, false
		}

		return int64(t), true
	case bool:
		if t {
			return 1, true
		}

		return 0, true
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)

		return n, err == nil
	}

	return 0, false
}

// toFloat coerces like pydantic's lax float. NaN and the infinities
// are refused: JSON cannot carry them, so storing one would fail.
func toFloat(v any) (float64, bool) {
	f, ok := laxFloat(v)
	if !ok || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}

	return f, true
}

func laxFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int64:
		return float64(t), true
	case int:
		return float64(t), true
	case bool:
		if t {
			return 1, true
		}

		return 0, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)

		return f, err == nil
	}

	return 0, false
}

var (
	trueWords  = []string{"1", "on", "t", "true", "y", "yes"}
	falseWords = []string{"0", "off", "f", "false", "n", "no"}
)

// toBool coerces like pydantic's lax bool.
func toBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case int64:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case int:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case string:
		w := strings.ToLower(strings.TrimSpace(t))
		if slices.Contains(trueWords, w) {
			return true, true
		}

		if slices.Contains(falseWords, w) {
			return false, true
		}
	}

	return false, false
}

// toList accepts the list shapes a JSON decode or a Go caller produces.
func toList(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}

		return out, true
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}

		return out, true
	}

	return nil, false
}

// cloneDefault copies a default so props never share it.
func cloneDefault(v any) any {
	if l, ok := v.([]any); ok {
		return slices.Clone(l)
	}

	return v
}
