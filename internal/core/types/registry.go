// Gosint, Copyright (c) 2026 YouSysAdmin

package types

import (
	"cmp"
	"errors"
	"slices"
	"strings"
	"sync"
)

// Categories group types in the add-node palette, in display order.
var Categories = []Category{
	{Key: "global", Title: "Global", Icon: "phrase"},
	{Key: "person", Title: "Identities & Entities", Icon: "individual"},
	{Key: "organization", Title: "Organization", Icon: "organization"},
	{Key: "contact", Title: "Communication & Contact", Icon: "phone"},
	{Key: "network", Title: "Network", Icon: "domain"},
	{Key: "security", Title: "Security & Access", Icon: "credential"},
	{Key: "files", Title: "Files & Documents", Icon: "file"},
	{Key: "financial", Title: "Financial Data", Icon: "creditcard"},
	{Key: "leaks", Title: "Leaks", Icon: "breach"},
	{Key: "crypto", Title: "Crypto", Icon: "cryptowallet"},
}

// Category is one palette group. Icon names the type whose icon it shows.
type Category struct {
	Key, Title, Icon string
}

// ErrNoLabel is props from which no label can be computed or taken.
var ErrNoLabel = errors.New("types: the value has no label")

// spec is how a type is declared in the catalog files.
type spec struct {
	name, title, category, icon, color string

	// before runs on the raw props, after on the field-checked ones.
	before, after func(props map[string]any) error
	label         func(props map[string]any) string
	fromString    func(s string) (map[string]any, error)
	detect        func(s string) bool
	key           func(props map[string]any) string
}

// Registry is a set of types by name: the built-in ones (base) and
// the custom ones layered over them. The maps and slices are replaced
// whole under mu when the custom set changes, readers take a copy.
type Registry struct {
	mu     sync.RWMutex
	byName map[string]*Type
	all    []*Type
	order  []*Type
	base   []*Type
	custom []*Type
}

// Builtin is the registry of every built-in type.
var Builtin = sync.OnceValue(func() *Registry {
	return newRegistry(catalog(), detectionOrder)
})

// detectionOrder is the order Detect tries types in, first match wins.
//
// Registration order, with two exceptions: CryptoWallet goes before
// Username, whose pattern would take every wallet address, and
// Individual goes last,
// because it takes any two or three words and must not shadow a more
// specific type.
//
// Gosint's credential and gitrepo go first: their patterns are strict
// (a known key prefix, an SSH algorithm, a forge URL), and a key would
// otherwise read as a username, a repository URL as a website.
var detectionOrder = []string{
	"cve", "credential", "gitrepo", "asn", "cidr", "creditcard", "domain", "email", "file", "ip", "phone", "port",
	"cryptowallet", "username", "website", "individual",
}

func newRegistry(specs []spec, order []string) *Registry {
	r := &Registry{byName: map[string]*Type{}}
	for _, s := range specs {
		t := &Type{
			Name:        s.name,
			Title:       s.title,
			Description: descriptions[s.name],
			Category:    s.category,
			Icon:        s.icon,
			Color:       s.color,
			Fields:      fieldTables[s.name],
			Label:       s.label,
			FromString:  s.fromString,
			Detect:      s.detect,
			Key:         s.key,
		}
		t.Normalize = r.normalizer(t, s.before, s.after)
		r.byName[s.name] = t
		r.all = append(r.all, t)
	}

	slices.SortStableFunc(r.all, byCategoryThenTitle)

	for _, name := range order {
		r.order = append(r.order, r.byName[name])
	}

	r.base = slices.Clone(r.all)

	return r
}

func (r *Registry) normalizer(t *Type, before, after func(map[string]any) error) func(map[string]any) error {
	return func(props map[string]any) error {
		if before != nil {
			if err := before(props); err != nil {
				return err
			}
		}

		if err := r.checkFields(t, props); err != nil {
			return err
		}

		if after != nil {
			return after(props)
		}

		return nil
	}
}

// Get returns a type by name, ignoring case.
func (r *Registry) Get(name string) (*Type, bool) {
	r.mu.RLock()
	t, ok := r.byName[strings.ToLower(name)]
	r.mu.RUnlock()

	return t, ok
}

// SecretFields names the fields of typeName marked Secret.
func (r *Registry) SecretFields(typeName string) []string {
	t, ok := r.Get(typeName)
	if !ok {
		return nil
	}

	var out []string
	for _, f := range t.Fields {
		if f.Secret {
			out = append(out, f.Name)
		}
	}

	return out
}

// All lists every type by category, then title.
func (r *Registry) All() []*Type {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return slices.Clone(r.all)
}

// byCategoryThenTitle orders types as the palette lists them: by the
// category's place in Categories, then by title.
func byCategoryThenTitle(a, b *Type) int {
	return cmp.Or(cmp.Compare(categoryRank(a.Category), categoryRank(b.Category)), cmp.Compare(a.Title, b.Title))
}

// categoryRank is the place of a category in Categories, first when it
// is not one.
func categoryRank(key string) int {
	return max(0, slices.IndexFunc(Categories, func(c Category) bool { return c.Key == key }))
}

// Detect returns the first type that recognizes the trimmed line.
func (r *Registry) Detect(line string) (*Type, bool) {
	line = pyStrip(line)
	if line == "" {
		return nil, false
	}

	r.mu.RLock()
	order := r.order
	r.mu.RUnlock()

	for _, t := range order {
		if t.Detect != nil && t.Detect(line) {
			return t, true
		}
	}

	return nil, false
}

// Parse detects the type of a line and builds normalized props from it.
func (r *Registry) Parse(line string) (*Type, map[string]any, error) {
	t, ok := r.Detect(line)
	if !ok {
		return nil, nil, errors.New("types: no type recognizes this value")
	}

	props, err := t.FromString(pyStrip(line))
	if err != nil {
		return nil, nil, err
	}

	if _, err := r.Build(t.Name, props); err != nil {
		return nil, nil, err
	}

	return t, props, nil
}

// Build normalizes props for typeName and returns the node label.
//
// A "label" or "nodeLabel" the caller put in props first fills an empty
// primary field, so a node typed in by its label alone is still checked
// by its type. A label computed from the props then wins, and the given
// one is the label only when the props compute none. Both keys are
// removed, since the label is the node's, not a property.
func (r *Registry) Build(typeName string, props map[string]any) (string, error) {
	t, ok := r.Get(typeName)
	if !ok {
		return "", errors.New("types: unknown type " + typeName)
	}

	provided := pyStrip(cmp.Or(str(props, "nodeLabel"), str(props, "label")))
	delete(props, "nodeLabel")
	delete(props, "label")

	if provided != "" {
		for _, f := range t.Fields {
			if f.Primary && textual(f.Kind) && !truthy(props[f.Name]) {
				props[f.Name] = provided
			}
		}
	}

	if err := t.Normalize(props); err != nil {
		return "", err
	}

	if label := pyStrip(t.Label(props)); label != "" {
		return label, nil
	}

	if provided == "" {
		return "", ErrNoLabel
	}

	return provided, nil
}

// Key is the identity of an entity within its type: the values of its
// primary fields, read after Build normalized them. "" when the type has
// no primary field set, and then the node falls back to its label.
//
// The label is display only and changes as properties are learned, an
// AS gaining its holder name for one, so it cannot be the identity.
func (r *Registry) Key(typeName string, props map[string]any) string {
	t, ok := r.Get(typeName)
	if !ok {
		return ""
	}

	if t.Key != nil {
		return pyStrip(t.Key(props))
	}

	var parts []string
	for _, f := range t.Fields {
		if !f.Primary {
			continue
		}

		v := props[f.Name]
		if m, isMap := v.(map[string]any); isMap && f.Ref != "" {
			v = r.Key(f.Ref, m)
		}

		if s := pyStrip(pyStr(v)); s != "" && v != nil {
			parts = append(parts, s)
		}
	}

	return strings.Join(parts, "|")
}

// textual reports whether a primary field of kind k can take a label.
func textual(k Kind) bool {
	switch k {
	case KindString, KindEmail, KindURL, KindAny:
		return true
	default:
		return false
	}
}

// checkFields coerces and checks every declared field of t in props.
func (r *Registry) checkFields(t *Type, props map[string]any) error {
	for _, f := range t.Fields {
		v, present := props[f.Name]
		if v == nil {
			if !present && f.Default != nil {
				props[f.Name] = cloneDefault(f.Default)

				continue
			}

			if f.Required {
				return fieldErr(t.Name, f.Name, "is required")
			}

			continue
		}

		out, err := r.coerce(f.Kind, f, v)
		if err != nil {
			return fieldErr(t.Name, f.Name, "%s", err)
		}

		props[f.Name] = out
	}

	return nil
}

// coerce checks one value against a field, returning its stored form.
func (r *Registry) coerce(kind Kind, f Field, v any) (any, error) {
	switch kind {
	case KindString, KindDatetime:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a string")
		}

		if len(f.Options) > 0 && !slices.Contains(f.Options, s) {
			return nil, errors.New("must be one of " + strings.Join(f.Options, ", "))
		}

		return s, nil
	case KindInt:
		n, ok := toInt(v)
		if !ok {
			return nil, errors.New("must be a whole number")
		}

		if (f.Min != nil && float64(n) < *f.Min) || (f.Max != nil && float64(n) > *f.Max) {
			return nil, errors.New("is out of range")
		}

		return n, nil
	case KindFloat:
		n, ok := toFloat(v)
		if !ok {
			return nil, errors.New("must be a number")
		}

		if (f.Min != nil && n < *f.Min) || (f.Max != nil && n > *f.Max) {
			return nil, errors.New("is out of range")
		}

		return n, nil
	case KindBool:
		b, ok := toBool(v)
		if !ok {
			return nil, errors.New("must be true or false")
		}

		return b, nil
	case KindURL:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be a URL")
		}

		return normalizeURL(s)
	case KindEmail:
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("must be an email address")
		}

		return normalizeEmail(s)
	case KindObject:
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errors.New("must be an object")
		}

		return m, nil
	case KindRef:
		return r.nested(f.Ref, v)
	case KindList:
		list, ok := toList(v)
		if !ok {
			return nil, errors.New("must be a list")
		}

		out := make([]any, 0, len(list))
		for _, e := range list {
			c, err := r.coerce(f.Elem, Field{Ref: f.Ref}, e)
			if err != nil {
				return nil, err
			}

			out = append(out, c)
		}

		return out, nil
	default:
		return v, nil
	}
}

// nested validates an embedded entity. A string is read with the
// nested type's FromString, so a bare phone number or organization
// name stands in for the object.
func (r *Registry) nested(typeName string, v any) (map[string]any, error) {
	t, ok := r.Get(typeName)
	if !ok {
		return nil, errors.New("unknown nested type " + typeName)
	}

	var props map[string]any
	switch x := v.(type) {
	case map[string]any:
		props = x
	case string:
		if t.FromString == nil {
			return nil, errors.New("must be an object")
		}

		p, err := t.FromString(x)
		if err != nil {
			return nil, err
		}

		props = p
	default:
		return nil, errors.New("must be an object")
	}

	if err := t.Normalize(props); err != nil {
		return nil, err
	}

	return props, nil
}
