// Gosint, Copyright (c) 2026 YouSysAdmin

// Package types is the registry of entity types a graph node can be:
// their fields, how a value is validated and normalized, how its label
// is computed, and how a line of text is recognized as one.
//
// Node properties stay JSON-shaped maps, so a type is data plus
// functions over a map rather than a Go struct, and unknown keys are
// kept.
package types

import "fmt"

// Kind is a field's value shape.
type Kind string

// The field kinds.
const (
	KindString   Kind = "string"
	KindInt      Kind = "int"
	KindFloat    Kind = "float"
	KindBool     Kind = "bool"
	KindDatetime Kind = "datetime"

	// KindURL is an http or https URL, stored normalized.
	KindURL Kind = "url"

	// KindEmail is an email address, stored normalized.
	KindEmail Kind = "email"
	KindList  Kind = "list"

	// KindObject is a free JSON object.
	KindObject Kind = "object"

	// KindRef is a nested entity of the type named by Field.Ref.
	KindRef Kind = "ref"

	// KindAny is any JSON value, unchecked.
	KindAny Kind = "any"
)

// Field is one property of a type.
type Field struct {
	Name, Title, Description string
	Kind                     Kind

	// Elem is the element kind when Kind is KindList.
	Elem Kind

	// Ref names the nested type when Kind or Elem is KindRef.
	Ref      string
	Required bool

	// Primary is the field a detected or typed-in value goes into.
	Primary bool

	// Long marks text that runs to several lines, so a form gives it
	// a text area.
	Long bool

	// Default is set when the field is absent.
	Default any

	// Options restrict a string to a fixed set, shown as a select.
	Options []string

	// VaultKey marks a string that names one of the account's vault
	// keys, never the secret itself, so a form offers the key names.
	VaultKey bool

	// Secret marks a value the graph store encrypts at rest and the
	// console masks until asked, a password or a found key.
	Secret bool

	// Min and Max bound a number.
	Min, Max *float64
}

// Type is one entity type.
type Type struct {
	// Name is the lowercase registry key, such as domain or cryptowallet.
	Name        string
	Title       string
	Description string
	Category    string

	// Icon is a lucide icon name, Color a hex color, "" for none.
	Icon   string
	Color  string
	Fields []Field

	// Normalize validates props and normalizes them in place, filling
	// computed fields. A FieldError names the bad field.
	Normalize func(props map[string]any) error

	// Label computes the node label from normalized props, "" when the
	// props give none.
	Label func(props map[string]any) string

	// FromString builds props from one line of text, nil when the type
	// has no textual form.
	FromString func(s string) (map[string]any, error)

	// Detect reports whether a trimmed line looks like this type, nil
	// when the type is never detected.
	Detect func(s string) bool

	// Key computes the entity's identity from normalized props, when
	// the primary field alone is not it. Nil for the primary field.
	Key func(props map[string]any) string
}

// FieldError is a value refused by a type.
type FieldError struct {
	Type    string
	Field   string
	Message string
}

// Error renders the refusal.
func (e *FieldError) Error() string {
	return fmt.Sprintf("%s.%s: %s", e.Type, e.Field, e.Message)
}

func fieldErr(typeName, field, format string, args ...any) error {
	return &FieldError{Type: typeName, Field: field, Message: fmt.Sprintf(format, args...)}
}
