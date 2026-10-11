// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// DecodeError is a body that could not be decoded into the request
// type, already phrased for the caller: the JSON key it concerns, a
// rule and a message that names no Go type.
type DecodeError struct {
	Field   string
	Rule    string
	Message string
	Err     error
}

func (e *DecodeError) Error() string { return e.Message }

func (e *DecodeError) Unwrap() error { return e.Err }

// decodeError explains a json/v2 failure against the type t the body
// was decoded into.
func decodeError(t reflect.Type, err error) *DecodeError {
	if se, ok := errors.AsType[*json.SemanticError](err); ok {
		if se.JSONPointer == "" {
			return &DecodeError{Rule: "type", Message: "Request body must be a JSON object", Err: err}
		}

		field := pointerField(t, string(se.JSONPointer))
		msg := friendlyField(field) + " must be " + expected(se.GoType)
		if se.Err != nil && strings.Contains(se.Err.Error(), "out of range") {
			msg = friendlyField(field) + " is out of range"
		}

		return &DecodeError{Field: field, Rule: "type", Message: msg, Err: err}
	}

	if se, ok := errors.AsType[*jsontext.SyntacticError](err); ok {
		if strings.Contains(se.Error(), "duplicate object member") {
			field := pointerField(t, string(se.JSONPointer))

			return &DecodeError{Field: field, Rule: "duplicate", Message: friendlyField(field) + " is given more than once", Err: err}
		}

	}

	return &DecodeError{Rule: "json", Message: "Request body is not valid JSON", Err: err}
}

// pointerField names the field a JSON pointer lands on the way the
// validator names it: the json name of the innermost struct field,
// list indexes skipped and map keys kept out, since a form has one
// control for a whole list or map.
func pointerField(t reflect.Type, ptr string) string {
	field := ""
	for raw := range strings.SplitSeq(strings.TrimPrefix(ptr, "/"), "/") {
		seg := strings.NewReplacer("~1", "/", "~0", "~").Replace(raw)
		for t != nil && t.Kind() == reflect.Pointer {
			t = t.Elem()
		}

		switch {
		case t == nil:
			return cmpField(field, seg)
		case t.Kind() == reflect.Slice || t.Kind() == reflect.Array:
			if _, err := strconv.Atoi(seg); err != nil {
				return cmpField(field, seg)
			}

			t = t.Elem()
		case t.Kind() == reflect.Map:
			return cmpField(field, seg)
		case t.Kind() == reflect.Struct:
			sf, ok := jsonField(t, seg)
			if !ok {
				return cmpField(field, seg)
			}

			field = seg
			t = sf.Type
		default:
			return cmpField(field, seg)
		}
	}

	return field
}

// cmpField falls back to the segment when no struct field was crossed.
func cmpField(field, seg string) string {
	if field != "" {
		return field
	}

	return seg
}

// jsonField finds the struct field a JSON member name decodes into,
// looking through embedded structs.
func jsonField(t reflect.Type, name string) (reflect.StructField, bool) {
	for sf := range t.Fields() {
		tag, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if tag == "-" {
			continue
		}

		if sf.Anonymous && tag == "" {
			et := sf.Type
			if et.Kind() == reflect.Pointer {
				et = et.Elem()
			}

			if et.Kind() == reflect.Struct {
				if inner, ok := jsonField(et, name); ok {
					return inner, true
				}
			}

			continue
		}

		if tag == name || (tag == "" && sf.Name == name) {
			return sf, true
		}
	}

	return reflect.StructField{}, false
}

var timeType = reflect.TypeFor[time.Time]()

// expected says in plain words what a Go type accepts.
func expected(t reflect.Type) string {
	if t == nil {
		return "a valid value"
	}

	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == timeType {
		return "a date and time (RFC 3339)"
	}

	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "an object"
	default:
		return "a valid value"
	}
}
