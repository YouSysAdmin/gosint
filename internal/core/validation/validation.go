// Gosint, Copyright (c) 2026 YouSysAdmin

// Package validation centralizes JSON-body parsing, normalization,
// and struct-tag validation for the API. Domain handlers declare
// their input shape with `validate:"..."` + `normalize:"..."` tags
// and call BindAndValidate - field-level errors come back through
// Humanize for response.BadRequestFields to render.
//
// Tag vocabulary used in this codebase:
//
//	validate:"required"                  field must be set/non-zero
//	validate:"min=N,max=N"               character-count bounds for strings,
//	                                     element-count bounds for slices/maps
//	validate:"oneof=a b c"               enum-style allowed values
//	validate:"required_if=Field val"     cross-field requirement
//	validate:"dive,..."                  apply remaining rules to each
//	                                     slice/map element
//
//	normalize:"trim"                     strings.TrimSpace
//	normalize:"lower"                    strings.ToLower
//	normalize:"upper"                    strings.ToUpper
//	normalize:"normalize"                trim + lower (the common pair)
//
// Example:
//
//	type loginRequest struct {
//	    Email    string `json:"email"    validate:"required,email" normalize:"normalize"`
//	    Password string `json:"password" validate:"required,min=1"  normalize:"trim"`
//	}
//
//	func (h Handler) Login(c fiber.Ctx) error {
//	    in, resp, ok := validation.Bind[loginRequest](c)
//	    if !ok {
//	        return resp
//	    }
//	    // ...
//	}
package validation

import (
	"bytes"
	"encoding/json/v2"
	"reflect"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
)

// build is the single construction of the validator. OnceValue holds
// the result, so there is no package-level variable a caller can read
// before it is written.
var build = sync.OnceValue(func() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	// Use the json tag name in error.Field() so error responses
	// match the JSON the SPA actually sent, not the Go field name.
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		tag := fld.Tag.Get("json")
		if tag == "" || tag == "-" {
			return fld.Name
		}

		tag, _, _ = strings.Cut(tag, ",")

		return tag
	})

	registerCustom(v)

	return v
})

// V returns the singleton *validator.Validate, built on first use.
func V() *validator.Validate {
	return build()
}

// Normalizer lets DTOs run custom normalization (cross-field
// coercion, defaulting, anything the normalize:"..." tag vocabulary
// doesn't cover) before struct-tag validation runs.
//
// Implement on a pointer receiver - BindAndValidate calls
// (&payload).Normalize(), so a value-receiver method won't fire.
type Normalizer interface {
	Normalize()
}

// fiberCtx is the slice of Fiber's fiber.Ctx that BindAndValidate
// needs. Declared as an interface so unit tests can stub it - in
// practice the only caller passes a real fiber.Ctx.
type fiberCtx interface {
	Body() []byte
}

// BindAndValidate parses the JSON body into T, runs normalization,
// then validates struct-tag rules. On a failure the error is returned
// with whatever was decoded so far, and the caller writes the
// response.
//
// Parsing is json/v2 with its default strictness, the same policy the
// app decoder carries (response.Unmarshal): a duplicate member, invalid
// UTF-8 or a mistyped value is refused rather than absorbed, and names
// match case-sensitively - a wrong-case key is ignored as unknown, the
// way an unknown key always was.
//
// Order matters:
//  1. JSON unmarshal so the struct exists.
//  2. normalize:"..." tag passes (trim/lower/etc) so a "  Foo  "
//     becomes "foo" before required / min / oneof see it.
//  3. Normalizer.Normalize() so DTO-specific coercion (default
//     fallbacks, cross-field defaulting) sees the cleaned strings.
//  4. validator.Struct so rules consult the final shape.
func BindAndValidate[T any](c fiberCtx) (T, error) {
	var payload T
	body := c.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		return payload, &DecodeError{Rule: "required", Message: "Request body is required"}
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, decodeError(reflect.TypeFor[T](), err)
	}

	applyNormalizeTags(reflect.ValueOf(&payload))
	if n, ok := any(&payload).(Normalizer); ok {
		n.Normalize()
	}

	if err := V().Struct(payload); err != nil {
		return payload, err
	}

	return payload, nil
}

// NormalizeAndValidate runs the same normalize-tag pass + Normalizer
// hook + validator.Struct check that BindAndValidate runs, but on a
// value that already exists in memory (i.e. wasn't decoded from a
// Fiber request body). The argument must be a non-nil pointer to a
// struct so the normalize-tag pass can mutate string fields in place.
//
// Use this when validating rows that arrived through a different
// channel than HTTP - e.g. a row nested inside a backup snapshot
// import body - so the same rules apply consistently to both paths.
func NormalizeAndValidate(payload any) error {
	applyNormalizeTags(reflect.ValueOf(payload))
	if n, ok := payload.(Normalizer); ok {
		n.Normalize()
	}

	return V().Struct(payload)
}

// applyNormalizeTags walks v and runs normalize:"..." tags on every
// settable string field, recursing into structs, slices and arrays.
// Map values are not reached, reflect cannot set them in place, so a
// map value needing normalization gets it in Normalize().
func applyNormalizeTags(v reflect.Value) {
	if !v.IsValid() {
		return
	}

	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}

		v = v.Elem()
	}

	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := range v.NumField() {
			fv := v.Field(i)
			ft := t.Field(i)
			applyNormalizeTags(fv)
			tag := ft.Tag.Get("normalize")
			if tag == "" || !fv.CanSet() {
				continue
			}

			switch {
			case fv.Kind() == reflect.String:
				fv.SetString(applyStringOps(fv.String(), tag))
			case (fv.Kind() == reflect.Slice || fv.Kind() == reflect.Array) && fv.Type().Elem().Kind() == reflect.String:
				// The tag sits on the field, the strings sit one level
				// down: a list of addresses is normalised element by
				// element or the tag on it means nothing.
				for j := range fv.Len() {
					el := fv.Index(j)
					el.SetString(applyStringOps(el.String(), tag))
				}
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			applyNormalizeTags(v.Index(i))
		}
	}
}

func applyStringOps(s string, tag string) string {
	for op := range strings.SplitSeq(tag, ",") {
		switch strings.ToLower(strings.TrimSpace(op)) {
		case "trim":
			s = strings.TrimSpace(s)
		case "lower":
			s = strings.ToLower(s)
		case "upper":
			s = strings.ToUpper(s)
		case "normalize":
			s = strings.TrimSpace(strings.ToLower(s))
		}
	}

	return s
}

// Compile-time guard: fiber.Ctx satisfies our minimal interface. If
// Fiber ever renames Body() the build breaks here, which is the
// right place to find out.
var _ fiberCtx = (fiber.Ctx)(nil)
