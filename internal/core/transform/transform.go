// Gosint, Copyright (c) 2026 YouSysAdmin

// Package transform is the transform framework: what a transform declares,
// what it is handed when it runs, and the worker handler that runs one
// as a scan.
//
// A transform takes nodes of one entity type and writes what it finds
// back into the sketch as nodes and edges, through Exec. Everything it
// writes goes through the type registry, so a value no type accepts is
// logged and skipped rather than stored.
package transform

import (
	"context"
	"slices"
	"strings"

	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// ParamType is how a parameter is entered and resolved.
type ParamType string

// The parameter types a transform declares.
const (
	ParamString ParamType = "string"
	ParamURL    ParamType = "url"
	ParamNumber ParamType = "number"
	ParamSelect ParamType = "select"

	// ParamBool is "true" or "false", a checkbox in the console.
	ParamBool ParamType = "bool"

	// ParamSecret is the name or id of a secret, resolved to its value
	// when the scan runs. Left empty, the key named like the parameter is
	// used.
	ParamSecret ParamType = "vaultSecret"
)

// Param is one setting a transform takes.
type Param struct {
	Name        string    `json:"name"`
	Type        ParamType `json:"type"`
	Description string    `json:"description"`
	Required    bool      `json:"required"`
	Default     string    `json:"default"`

	// Options are the choices of a select.
	Options []Option `json:"options"`
}

// Option is one choice of a select parameter.
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// OnOff is the choices of a select that switches something.
var OnOff = []Option{{Label: "Enabled", Value: "true"}, {Label: "Disabled", Value: "false"}}

// Meta is what a transform declares about itself.
type Meta struct {
	// Name is the stable key, such as domain_to_ip.
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Icon        string `json:"icon"`

	// Input and Output are entity type names. Input "" takes any type.
	Input  string  `json:"input"`
	Output string  `json:"output"`
	Params []Param `json:"params"`
}

// Transform is one lookup run on entities, which writes what it finds.
type Transform interface {
	Meta() Meta

	// Run enriches inputs, nodes of the sketch already of Meta().Input.
	// A failure on one input is logged and the rest go on. An error ends
	// the scan as failed.
	Run(ctx context.Context, x *Exec, inputs []graphmodel.Node) error
}

// Registry holds the transforms by name: the built-in ones, and those
// a source hands over when asked, such as the files of a directory an
// operator edits while the server runs.
type Registry struct {
	byName map[string]Transform
	all    []Transform
	source func() []Transform
}

// NewRegistry builds a registry, panicking on a duplicate name, which
// is a programming error.
func NewRegistry(transforms ...Transform) *Registry {
	r := &Registry{byName: map[string]Transform{}}
	for _, e := range transforms {
		name := e.Meta().Name
		if _, dup := r.byName[name]; dup {
			panic("transform: duplicate transform " + name)
		}

		r.byName[name] = e
		r.all = append(r.all, e)
	}

	slices.SortFunc(r.all, func(a, b Transform) int { return strings.Compare(a.Meta().Name, b.Meta().Name) })

	return r
}

// SetSource adds transforms found on each call, after the built-in
// ones. A built-in name wins over the source's, which is left out.
func (r *Registry) SetSource(source func() []Transform) { r.source = source }

// Get returns a transform by name.
func (r *Registry) Get(name string) (Transform, bool) {
	if e, ok := r.byName[name]; ok {
		return e, true
	}

	if r.source != nil {
		for _, e := range r.source() {
			if e.Meta().Name == name {
				return e, true
			}
		}
	}

	return nil, false
}

// For lists the transforms taking typeName, or all of them for "".
func (r *Registry) For(typeName string) []Transform {
	all := r.all
	if r.source != nil {
		all = slices.Clone(r.all)
		for _, e := range r.source() {
			if _, dup := r.byName[e.Meta().Name]; !dup {
				all = append(all, e)
			}
		}
	}

	if typeName == "" {
		return slices.Clone(all)
	}

	var out []Transform
	for _, e := range all {
		if in := e.Meta().Input; in == "" || in == typeName {
			out = append(out, e)
		}
	}

	return out
}
