// Gosint, Copyright (c) 2026 YouSysAdmin

package response

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
)

// marshalOptions is the wire policy, stated once. A nil slice is []
// (the json/v2 default), a nil map stays null, and invalid UTF-8 taken
// from scanned content is coerced instead of failing the response.
var marshalOptions = json.JoinOptions(
	json.FormatNilMapAsNull(true),
	jsontext.AllowInvalidUTF8(true),
)

// Marshal renders v under the wire policy.
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v, marshalOptions)
}

// Unmarshal parses request JSON under json/v2's strict defaults.
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
