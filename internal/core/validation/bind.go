// Gosint, Copyright (c) 2026 YouSysAdmin

package validation

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"reflect"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/response"
)

// Bind parses and validates the request body, writing the 400 itself
// when the body does not hold up.
//
// The one place that knows how a validation error becomes a response
// - Humanize, then Summary, then which helper - so the error envelope
// is decided here and not in every handler.
//
// Three return values, like the auth middleware: the response helpers
// write the status and return nil, so an error alone cannot tell
// "rejected" from "fine". Return resp the moment ok is false:
//
//	in, resp, ok := validation.Bind[createInput](c)
//	if !ok {
//	    return resp
//	}
func Bind[T any](c fiber.Ctx) (out T, resp error, ok bool) {
	in, err := BindAndValidate[T](c)
	if err != nil {
		return out, refuse(c, err), false
	}

	return in, nil, true
}

// BindOptional is Bind for a route whose body may be left out: an empty
// body is the zero value, validated like any other.
func BindOptional[T any](c fiber.Ctx) (out T, resp error, ok bool) {
	if len(bytes.TrimSpace(c.Body())) > 0 {
		return Bind[T](c)
	}

	if err := NormalizeAndValidate(&out); err != nil {
		return out, refuse(c, err), false
	}

	return out, nil, true
}

// BindOnto is Bind for a partial update. The body is decoded over base,
// so a field the body leaves out keeps the value base carries, and a
// top-level map or list the body names replaces the one in base rather
// than merging into it. One nested deeper is merged by json/v2.
func BindOnto[T any](c fiber.Ctx, base T) (out T, resp error, ok bool) {
	in, err := decodeOnto(c.Body(), base)
	if err != nil {
		return out, refuse(c, err), false
	}

	return in, nil, true
}

// decodeOnto is BindOnto short of the response.
func decodeOnto[T any](body []byte, base T) (T, error) {
	var named map[string]jsontext.Value
	if len(bytes.TrimSpace(body)) == 0 || json.Unmarshal(body, &named) != nil {
		return BindAndValidate[T](bodyOnly(body))
	}

	clearNamedCollections(reflect.ValueOf(&base).Elem(), named)
	if err := json.Unmarshal(body, &base); err != nil {
		return base, decodeError(reflect.TypeFor[T](), err)
	}

	if err := NormalizeAndValidate(&base); err != nil {
		return base, err
	}

	return base, nil
}

// bodyOnly hands a body already read to BindAndValidate.
type bodyOnly []byte

func (b bodyOnly) Body() []byte { return b }

// clearNamedCollections zeroes every map and slice field of v whose json
// name the body carries, since json/v2 merges into what is there.
func clearNamedCollections(v reflect.Value, named map[string]jsontext.Value) {
	if v.Kind() != reflect.Struct {
		return
	}

	t := v.Type()
	for i := range t.NumField() {
		f := t.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if !f.IsExported() || name == "" || name == "-" {
			continue
		}

		if _, ok := named[name]; !ok {
			continue
		}

		if k := f.Type.Kind(); k == reflect.Map || k == reflect.Slice {
			v.Field(i).SetZero()
		}
	}
}

// refuse writes the 400 for a decode or validation failure.
func refuse(c fiber.Ctx, err error) error {
	fes := Humanize(err)

	return response.BadRequestFields(c, Summary(fes), fes)
}
