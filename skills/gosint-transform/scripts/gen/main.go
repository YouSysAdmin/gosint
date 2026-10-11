// Gosint, Copyright (c) 2026 YouSysAdmin

// Command gen refreshes what the skill copies from the code: the
// catalog inside validate.py (entity types, built-in transform names)
// and references/entity-types.md. Run from the repository root:
//
//	go run ./skills/gosint-transform/scripts/gen
package main

import (
	"bytes"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/transforms"
)

const dir = "skills/gosint-transform"

const (
	beginMark = "# BEGIN CATALOG"
	endMark   = "# END CATALOG"
)

// entityType is one type as the validator reads it.
type entityType struct {
	Primary string            `json:"primary"`
	Fields  map[string]string `json:"fields"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	all := types.Builtin().All()
	catalog := map[string]entityType{}
	var table strings.Builder
	for _, t := range all {
		et := entityType{Fields: map[string]string{}}
		var fields []string
		for _, f := range t.Fields {
			k := kind(f)
			et.Fields[f.Name] = k
			fields = append(fields, f.Name+" "+k)
			if f.Primary {
				et.Primary = f.Name
			}
		}

		catalog[t.Name] = et
		fmt.Fprintf(&table, "| `%s` | %s | `%s` | %s |\n", t.Name, t.Category, et.Primary, strings.Join(fields, ", "))
	}

	var builtin []string
	for _, t := range transforms.All() {
		builtin = append(builtin, t.Meta().Name)
	}

	slices.Sort(builtin)

	if err := writeCatalog(catalog, builtin); err != nil {
		return err
	}

	return writeTable(table.String())
}

// kind is a field's kind as the reference writes it.
func kind(f types.Field) string {
	k := string(f.Kind)
	if f.Kind == types.KindRef {
		k = "ref:" + f.Ref
	}

	if f.Kind == types.KindList {
		e := string(f.Elem)
		if f.Elem == types.KindRef {
			e = "ref:" + f.Ref
		}

		k = "list<" + e + ">"
	}

	return k
}

func writeCatalog(catalog map[string]entityType, builtin []string) error {
	path := filepath.Join(dir, "scripts", "validate.py")
	src, err := os.ReadFile(path) //nolint:gosec // a file of this repository
	if err != nil {
		return err
	}

	start := bytes.Index(src, []byte(beginMark))
	end := bytes.Index(src, []byte(endMark))
	if start < 0 || end < start {
		return fmt.Errorf("%s has no %s ... %s block", path, beginMark, endMark)
	}

	typesJSON, err := json.Marshal(catalog, json.Deterministic(true))
	if err != nil {
		return err
	}

	namesJSON, err := json.Marshal(builtin)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	b.Write(src[:start])
	b.WriteString(beginMark + " (written by scripts/gen, do not edit)\n")
	fmt.Fprintf(&b, "TYPES = json.loads(r'''%s''')\n", typesJSON)
	fmt.Fprintf(&b, "BUILTIN = json.loads(r'''%s''')\n", namesJSON)
	b.Write(src[end:])

	return os.WriteFile(path, b.Bytes(), 0o644) //nolint:gosec // a script of this repository
}

func writeTable(rows string) error {
	path := filepath.Join(dir, "references", "entity-types.md")
	src, err := os.ReadFile(path) //nolint:gosec // a file of this repository
	if err != nil {
		return err
	}

	const head = "|---|---|---|---|\n"
	i := bytes.Index(src, []byte(head))
	if i < 0 {
		return fmt.Errorf("%s has no table", path)
	}

	out := append(src[:i+len(head):i+len(head)], rows...)

	return os.WriteFile(path, out, 0o644) //nolint:gosec // a file of this repository
}
