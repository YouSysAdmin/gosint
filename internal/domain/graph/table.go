// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/tabular"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/core/validation"
)

// A CSV or XLSX file imports by a MAPPING the person draws in the
// console: each row of the table makes the entities the mapping names,
// a column per field, and the relationships between them. An entity
// whose primary field is empty on a row is skipped for that row, and
// the same value on two rows is one entity, by the type's key.

// maxTable bounds the file.
const maxTable = 16 << 20

// previewRows is how many rows the preview shows.
const previewRows = 20

// sampleValues is how many values of a column the type guess reads.
const sampleValues = 50

// tableEntity is one entity each row makes: its type and which column
// fills which field.
type tableEntity struct {
	Type   string            `json:"type"   validate:"required,nodetype"`
	Fields map[string]string `json:"fields" validate:"required,min=1,max=64"`
}

// tableLink joins two of the row's entities, by their index in the
// mapping, under a label.
type tableLink struct {
	From  int    `json:"from"  validate:"min=0,max=19"`
	To    int    `json:"to"    validate:"min=0,max=19"`
	Label string `json:"label" validate:"required,max=128" normalize:"trim"`
}

// tableMapping is the mapping, sent as the multipart field "mapping".
type tableMapping struct {
	Entities []tableEntity `json:"entities" validate:"required,min=1,max=20,dive"`
	Links    []tableLink   `json:"links"    validate:"max=40,dive"`
}

// columnGuess is the type most of a column's sampled values look like.
type columnGuess struct {
	Column string `json:"column"`
	Type   string `json:"type"`
}

// tablePreview answers the preview: the columns, the first rows, how
// many there are, and a type guess per column.
type tablePreview struct {
	Columns []string      `json:"columns"`
	Rows    [][]string    `json:"rows"`
	Total   int           `json:"total"`
	Guesses []columnGuess `json:"guesses"`
}

var listSeparators = regexp.MustCompile(`\s*[;|\n]\s*`)

// readTable reads the multipart file as a table.
func readTable(c fiber.Ctx) (*tabular.Table, error) {
	fh, err := c.FormFile("file")
	if err != nil {
		return nil, errors.New("send the table as the multipart field named file")
	}

	if fh.Size > maxTable {
		return nil, errors.New("the file is over 16 MB")
	}

	f, err := fh.Open()
	if err != nil {
		return nil, err
	}

	defer f.Close() //nolint:errcheck // read-only

	data, err := io.ReadAll(io.LimitReader(f, maxTable))
	if err != nil {
		return nil, err
	}

	return tabular.Read(fh.Filename, data)
}

// ImportTablePreview reads a CSV or XLSX file and answers its shape, so
// the console can offer the mapping.
func (h Handler) ImportTablePreview(c fiber.Ctx) error {
	t, err := readTable(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	out := tablePreview{Columns: t.Columns, Rows: t.Rows[:min(len(t.Rows), previewRows)], Total: len(t.Rows), Guesses: []columnGuess{}}
	reg := types.Builtin()
	for i, col := range t.Columns {
		counts := map[string]int{}
		sampled := 0
		for _, row := range t.Rows {
			if sampled == sampleValues {
				break
			}

			if row[i] == "" {
				continue
			}

			sampled++
			if typ, ok := reg.Detect(row[i]); ok {
				counts[typ.Name]++
			}
		}

		// A guess needs most of the sample to agree, and the fallback
		// (a phrase) says nothing.
		for name, n := range counts {
			if name != "phrase" && n*10 >= sampled*6 {
				out.Guesses = append(out.Guesses, columnGuess{Column: col, Type: name})

				break
			}
		}
	}

	return response.Success(c, out)
}

// ImportTable imports a CSV or XLSX file by a mapping.
func (h Handler) ImportTable(c fiber.Ctx) error {
	var m tableMapping
	if err := response.Unmarshal([]byte(c.FormValue("mapping")), &m); err != nil {
		return response.BadRequest(c, "mapping is not JSON")
	}

	if err := validation.NormalizeAndValidate(&m); err != nil {
		return response.BadRequest(c, "mapping: "+validation.Summary(validation.Humanize(err)))
	}

	t, err := readTable(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	in, err := tableInput(t, m)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	if len(in.Nodes) == 0 {
		return response.BadRequest(c, "no row fills the main field of any entity")
	}

	return h.runImport(c, in)
}

// tableInput turns the table into an import by the mapping.
func tableInput(t *tabular.Table, m tableMapping) (importInput, error) {
	in := importInput{Source: "table"}
	index := map[string]int{}
	for i, col := range t.Columns {
		index[col] = i
	}

	reg := types.Builtin()
	for ei, e := range m.Entities {
		typ, ok := reg.Get(e.Type)
		if !ok {
			return in, fmt.Errorf("entity %d: unknown type %q", ei+1, e.Type)
		}

		for field, col := range e.Fields {
			if _, ok := index[col]; !ok {
				return in, fmt.Errorf("entity %d: the table has no column %q", ei+1, col)
			}

			if fieldOf(typ, field) == nil {
				return in, fmt.Errorf("entity %d: %s has no field %q", ei+1, e.Type, field)
			}
		}
	}

	for _, l := range m.Links {
		if l.From >= len(m.Entities) || l.To >= len(m.Entities) || l.From == l.To {
			return in, errors.New("a link names an entity the mapping does not have")
		}
	}

	for ri, row := range t.Rows {
		ids := make([]string, len(m.Entities))
		for ei, e := range m.Entities {
			typ, _ := reg.Get(e.Type)
			props := map[string]any{}
			for field, col := range e.Fields {
				v := row[index[col]]
				if v == "" {
					continue
				}

				props[field] = cellValue(fieldOf(typ, field), v)
			}

			if !primaryFilled(typ, props) {
				continue
			}

			id := fmt.Sprintf("r%de%d", ri+1, ei+1)
			ids[ei] = id
			in.Nodes = append(in.Nodes, importNode{
				ID:         id,
				Type:       e.Type,
				Label:      primaryValue(typ, props),
				Properties: props,
			})
		}

		for _, l := range m.Links {
			if ids[l.From] != "" && ids[l.To] != "" {
				in.Edges = append(in.Edges, importEdge{Source: ids[l.From], Target: ids[l.To], Label: l.Label})
			}
		}
	}

	return in, nil
}

func fieldOf(t *types.Type, name string) *types.Field {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i]
		}
	}

	return nil
}

// cellValue is the cell as the field takes it: a list split on
// semicolons, pipes or line breaks (a comma may sit inside a value),
// anything else as text for the type to coerce.
func cellValue(f *types.Field, v string) any {
	if f.Kind != types.KindList {
		return v
	}

	parts := listSeparators.Split(v, -1)
	out := make([]any, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}

	return out
}

// primaryFilled says whether the row gave the entity its main field,
// or any value when the type has none.
// primaryValue is the primary field's value as text, what a row's entity
// is called before its type builds the label: a refusal names it.
func primaryValue(t *types.Type, props map[string]any) string {
	for _, f := range t.Fields {
		if f.Primary {
			if v, ok := props[f.Name]; ok && v != "" {
				return fmt.Sprint(v)
			}
		}
	}

	return ""
}

func primaryFilled(t *types.Type, props map[string]any) bool {
	hasPrimary := false
	for _, f := range t.Fields {
		if f.Primary {
			hasPrimary = true
			if v, ok := props[f.Name]; ok && v != "" {
				return true
			}
		}
	}

	return !hasPrimary && len(props) > 0
}
