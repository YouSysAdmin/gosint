// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"

	"github.com/go-playground/validator/v10"
)

// FieldError is the normalized error item the SPA renders next to
// the offending input. JSON-stable - the SPA's fields[].field matches
// the json tag the user actually sent.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Humanize converts validator.ValidationErrors into []FieldError. A
// body that did not decode reports the key it concerns. Any other
// error is the server's own, so the client gets a plain refusal and
// the detail goes to the log.
func Humanize(err error) []FieldError {
	if de, ok := errors.AsType[*DecodeError](err); ok {
		return []FieldError{{Field: de.Field, Rule: de.Rule, Message: de.Message}}
	}

	ve, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		slog.Warn("validation: unexpected error", "err", err)

		return []FieldError{{Message: "invalid request body"}}
	}

	out := make([]FieldError, 0, len(ve))
	for _, fe := range ve {
		out = append(out, FieldError{
			Field:   listField(fe.Field()),
			Rule:    fe.Tag(),
			Message: defaultMessage(fe),
		})
	}

	slices.SortFunc(out, func(a, b FieldError) int { return cmp.Compare(a.Field, b.Field) })

	return out
}

// listField keys an element's error on the list it belongs to. A dive
// rule reports `addresses[2]`, and a form has one control for the whole
// list, so that is the name it looks the error up under. The message
// keeps the index and says which entry.
func listField(name string) string {
	if before, _, ok := strings.Cut(name, "["); ok && before != "" {
		return before
	}

	return name
}

// Summary collapses []FieldError into one human line, suitable for
// the response envelope's "error" string when the SPA isn't yet
// consuming the structured "fields" array.
func Summary(fes []FieldError) string {
	if len(fes) == 0 {
		return "validation failed"
	}

	if len(fes) == 1 {
		return fes[0].Message
	}

	parts := make([]string, 0, len(fes))
	for _, fe := range fes {
		parts = append(parts, fe.Message)
	}

	return "validation failed: " + strings.Join(parts, ". ")
}

// sizeUnit names what min, max and len count. The tag is the same for
// a number and a length, so the unit comes from the field's kind: a
// string counts characters, a list counts items, a number is bare.
func sizeUnit(fe validator.FieldError) string {
	unit := ""
	switch fe.Kind() {
	case reflect.String:
		unit = " character"
	case reflect.Slice, reflect.Array, reflect.Map:
		unit = " item"
	}

	if unit != "" && fe.Param() != "1" {
		unit += "s"
	}

	return unit
}

func defaultMessage(fe validator.FieldError) string {
	field := friendlyField(fe.Field())
	switch fe.Tag() {
	case "required":
		return field + " is required"
	case "required_if", "required_unless", "required_with", "required_without":
		return field + " is required when other fields change"
	case "email":
		return field + " must be a valid email"
	case "nodetype":
		return field + " must be a lowercase type name such as domain or ip"
	case "keyname":
		return field + " may contain letters, digits, dot, dash and underscore"
	case "notblank":
		return field + " must not be blank"
	case "maxbytes":
		return fmt.Sprintf("%s must be at most %s bytes", field, fe.Param())
	case "ip":
		return field + " must be an IP address"
	case "alpha":
		return field + " must be letters only"
	case "timezone":
		return field + " must be an IANA timezone such as Europe/Lisbon"
	case "uuid", "uuid4", "uuid5":
		return field + " must be a valid UUID"
	case "min":
		return fmt.Sprintf("%s must be at least %s%s", field, fe.Param(), sizeUnit(fe))
	case "max":
		return fmt.Sprintf("%s must be at most %s%s", field, fe.Param(), sizeUnit(fe))
	case "oneof":
		return fmt.Sprintf("%s must be one of: %s", field, strings.ReplaceAll(fe.Param(), " ", ", "))
	case "len":
		return fmt.Sprintf("%s must be exactly %s%s", field, fe.Param(), sizeUnit(fe))
	case "url":
		return field + " must be a URL"
	case "fqdn":
		return field + " must be a domain name"
	case "hostname_rfc1123":
		return field + " must be a hostname"
	case "json":
		return field + " must be valid JSON"
	case "numeric":
		return field + " must be a number"
	case "startswith":
		return fmt.Sprintf("%s must start with %s", field, fe.Param())
	case "eqfield":
		return fmt.Sprintf("%s must match %s", field, friendlyField(fe.Param()))
	default:
		// Fallback: validator's library message with quotes stripped.
		return strings.ReplaceAll(fe.Error(), "\"", "")
	}
}

// friendlyLabels overrides the auto-titlecase for json tags that
// expand to unidiomatic English (e.g. "smtp_host" -> "SMTP host").
// Add an entry here when a new field's auto-rendered form reads
// awkwardly in error messages.
var friendlyLabels = map[string]string{
	"id": "ID",
}

// friendlyWords are the abbreviations a label spells in capitals
// wherever they appear in it.
var friendlyWords = map[string]string{
	"id": "ID", "ids": "IDs", "smtp": "SMTP", "url": "URL", "ip": "IP", "ips": "IPs",
	"asn": "ASN", "cidr": "CIDR", "dns": "DNS", "json": "JSON",
}

// friendlyField converts a json tag name to a human-readable label
// suitable for an end-user-facing error message. Known cases come
// from the friendlyLabels map - everything else falls through to a
// sentence-cased version with underscores replaced by spaces
// ("max_concurrent_runners" -> "Max concurrent runners").
//
// The structured FieldError envelope keeps the original json tag in
// FieldError.Field so the SPA can still route the error to the
// matching input - only the Message is humanized.
func friendlyField(jsonField string) string {
	if jsonField == "" {
		return "This field"
	}

	if v, ok := friendlyLabels[jsonField]; ok {
		return v
	}

	parts := strings.Split(jsonField, "_")
	for i, p := range parts {
		if w, ok := friendlyWords[p]; ok {
			parts[i] = w

			continue
		}

		if i == 0 && len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}

	return strings.Join(parts, " ")
}
