// Gosint, Copyright (c) 2026 YouSysAdmin

package database

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rebind converts `?` placeholders into postgres's $1..$N.
//
// The scan is textual and does not skip string literals, so store SQL
// never carries a literal question mark.
func Rebind(query string) string {
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))

			continue
		}

		b.WriteRune(r)
	}

	return b.String()
}

// NullStr returns nil for an empty string so it lands as SQL NULL.
func NullStr(s string) any {
	if s == "" {
		return nil
	}

	return s
}

// Str is the read half of NullStr: a scan target that turns SQL NULL
// into the empty string.
func Str(dst *string) any { return nullStrScanner{dst} }

type nullStrScanner struct{ dst *string }

// Scan implements sql.Scanner.
func (n nullStrScanner) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		*n.dst = ""
	case string:
		*n.dst = t
	case []byte:
		*n.dst = string(t)
	default:
		return fmt.Errorf("database.Str: cannot scan %T into a string", v)
	}

	return nil
}

// NullTime returns nil when t is nil, otherwise the value.
func NullTime(t *time.Time) any {
	if t == nil {
		return nil
	}

	return *t
}

// EscapeLike neutralizes LIKE metacharacters, for `LIKE ? ESCAPE '\'`.
func EscapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)

	return strings.ReplaceAll(s, "_", `\_`)
}

// storedJSONOptions is how a Go value becomes column text. Deterministic
// so the same value always yields the same bytes, and invalid UTF-8 is
// coerced because postgres refuses it in a TEXT column.
var storedJSONOptions = json.JoinOptions(
	json.Deterministic(true),
	jsontext.AllowInvalidUTF8(true),
)

// MustJSON marshals v for a JSON TEXT column and panics on error, which
// only a programmer bug (a chan or func in a model) can cause.
func MustJSON(v any) string {
	b, err := json.Marshal(v, storedJSONOptions)
	if err != nil {
		// The type only: the value may hold an opened secret field.
		panic(fmt.Sprintf("database.MustJSON: %v (input: %T)", err, v))
	}

	return string(b)
}

// UnmarshalJSON parses a JSON TEXT column into v. Empty input is a
// no-op, so a column added later reads as the zero value.
func UnmarshalJSON(raw string, v any) error {
	if raw == "" {
		return nil
	}

	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("decode stored json: %w", err)
	}

	return nil
}
