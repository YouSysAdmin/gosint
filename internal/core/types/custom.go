// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A custom type is one an administrator declares as data: a name, a
// title, a category, an icon and a colour, and fields with no hooks.
// Its label is its primary field, its identity too, it is never
// detected from text, and a line typed in fills the primary field.
// The registry layers the custom types over the built-in ones, so
// every caller of Builtin sees them, and replaces the set whole.

// CustomType is a type declared as data.
type CustomType struct {
	Name        string        `json:"name"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	Category    string        `json:"category"`
	Icon        string        `json:"icon"`
	Color       string        `json:"color"`
	Fields      []CustomField `json:"fields"`
}

// CustomField is one field of a custom type.
type CustomField struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Required    bool     `json:"required"`
	Primary     bool     `json:"primary"`
	Long        bool     `json:"long"`
	Options     []string `json:"options"`
}

var (
	customNameRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)
	customKinds   = []Kind{KindString, KindInt, KindFloat, KindBool, KindDatetime, KindURL, KindEmail, KindList}
	customColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

	// customIconRe is the shape of a Lucide icon name. The list of the
	// bundled ones is the console's, which draws an unknown one as a
	// plain circle.
	customIconRe = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,47}$`)

	// reservedFields are property keys Build takes as the given label,
	// so a field of that name would never be stored.
	reservedFields = []string{"label", "nodelabel"}
)

// CheckCustom says whether def can become a type beside the built-in
// ones: a name of its own, one primary text field, known kinds.
func (r *Registry) CheckCustom(def CustomType) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if !customNameRe.MatchString(def.Name) {
		fail("name must be lowercase letters, digits and underscores, starting with a letter")
	} else if r.builtin(def.Name) {
		fail("%s is a built-in type", def.Name)
	}

	if strings.TrimSpace(def.Title) == "" {
		fail("title is required")
	}

	if !slices.ContainsFunc(Categories, func(c Category) bool { return c.Key == def.Category }) {
		fail("category %q is not one of the palette's", def.Category)
	}

	if def.Color != "" && !customColorRe.MatchString(def.Color) {
		fail("color must be #RRGGBB")
	}

	if def.Icon != "" && !customIconRe.MatchString(def.Icon) {
		fail("icon must be the name of a bundled icon, such as Tag")
	}

	if len(def.Fields) == 0 {
		fail("a type needs at least one field")
	}

	primaries, seen := 0, map[string]bool{}
	for _, f := range def.Fields {
		switch {
		case !customNameRe.MatchString(f.Name):
			fail("field %q: a name is lowercase letters, digits and underscores", f.Name)
		case slices.Contains(reservedFields, f.Name):
			fail("field %q: the name is the entity's label, pick another", f.Name)
		case seen[f.Name]:
			fail("field %q is declared twice", f.Name)
		case !slices.Contains(customKinds, Kind(f.Kind)):
			fail("field %q: unknown kind %q", f.Name, f.Kind)
		case f.Primary && Kind(f.Kind) != KindString && Kind(f.Kind) != KindURL && Kind(f.Kind) != KindEmail:
			fail("field %q: the primary field holds text", f.Name)
		case len(f.Options) > 0 && Kind(f.Kind) != KindString:
			fail("field %q: only a text field has options", f.Name)
		}

		seen[f.Name] = true
		if f.Primary {
			primaries++
		}
	}

	if primaries != 1 {
		fail("exactly one field is the primary one, the entity's name")
	}

	return errors.Join(errs...)
}

// SetCustom replaces the custom types with defs, each checked. The
// built-in ones stay as they are.
func (r *Registry) SetCustom(defs []CustomType) error {
	built := make([]*Type, 0, len(defs))
	for _, def := range defs {
		if err := r.CheckCustom(def); err != nil {
			return fmt.Errorf("type %s: %w", def.Name, err)
		}

		built = append(built, r.customType(def))
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	byName := make(map[string]*Type, len(r.base)+len(built))
	all := make([]*Type, 0, len(r.base)+len(built))
	for _, t := range r.base {
		byName[t.Name] = t
		all = append(all, t)
	}

	for _, t := range built {
		if _, dup := byName[t.Name]; dup {
			return fmt.Errorf("type %s is declared twice", t.Name)
		}

		byName[t.Name] = t
		all = append(all, t)
	}

	r.byName, r.all = byName, sorted(all)
	r.custom = built

	return nil
}

// Custom lists the custom types in place.
func (r *Registry) Custom() []*Type {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.custom)
}

func (r *Registry) builtin(name string) bool {
	return slices.ContainsFunc(r.base, func(t *Type) bool { return t.Name == name })
}

// customType builds the type: the generic field check, the primary
// field as the label and as what a typed line fills.
func (r *Registry) customType(def CustomType) *Type {
	fields := make([]Field, 0, len(def.Fields))
	primary := ""
	for _, f := range def.Fields {
		field := Field{Name: f.Name, Title: f.Title, Description: f.Description, Kind: Kind(f.Kind),
			Required: f.Required, Primary: f.Primary, Long: f.Long, Options: f.Options}
		if field.Title == "" {
			field.Title = strings.ReplaceAll(f.Name, "_", " ")
		}

		if field.Kind == KindList {
			field.Elem = KindString
		}

		if f.Primary {
			primary = f.Name
			field.Required = true
		}

		fields = append(fields, field)
	}

	t := &Type{Name: def.Name, Title: def.Title, Description: def.Description, Category: def.Category,
		Icon: def.Icon, Color: def.Color, Fields: fields}
	t.Normalize = r.normalizer(t, nil, nil)
	t.Label = func(props map[string]any) string { return str(props, primary) }
	t.FromString = func(s string) (map[string]any, error) {
		return map[string]any{primary: strings.TrimSpace(s)}, nil
	}

	return t
}

// sorted orders types by category rank, then title.
func sorted(all []*Type) []*Type {
	slices.SortStableFunc(all, byCategoryThenTitle)

	return all
}
