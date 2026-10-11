// Gosint, Copyright (c) 2026 YouSysAdmin

package flow

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	coreflow "github.com/yousysadmin/gosint/internal/core/flow"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	flowmodel "github.com/yousysadmin/gosint/internal/models/flow"
)

// A flow moves between installs as one JSON document, or several in
// one, with no id: the import makes new flows the caller owns. A flow
// naming a transform this install lacks, or a name the caller already
// has, is skipped and said so.

const (
	kindFlow  = "flow"
	kindFlows = "flows"

	// maxFlowFile bounds an import.
	maxFlowFile = 4 << 20
)

// flowDocument is one exported flow.
type flowDocument struct {
	Name        string               `json:"name"        validate:"required,notblank,max=200" normalize:"trim"`
	Description string               `json:"description" validate:"max=10000" normalize:"trim"`
	Definition  flowmodel.Definition `json:"definition"`
}

// exportDocument is the file: one flow, or a list of them.
type exportDocument struct {
	Gosint  string         `json:"gosint"`
	Version int            `json:"version"`
	Flow    *flowDocument  `json:"flow,omitempty"`
	Flows   []flowDocument `json:"flows,omitempty"`
}

// importResponse says what an import did.
type importResponse struct {
	Imported int              `json:"imported"`
	Flows    []flowmodel.Flow `json:"flows"`
	Skipped  []string         `json:"skipped"`
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func fileName(title, fallback string) string {
	name := strings.Trim(unsafeName.ReplaceAllString(title, "_"), "_. ")
	if name == "" {
		return fallback
	}

	return name
}

func document(f flowmodel.Flow) flowDocument {
	return flowDocument{Name: f.Name, Description: f.Description, Definition: f.Definition}
}

// Export answers one of the caller's flows as a file.
func (h Handler) Export(c fiber.Ctx) error {
	f, err := h.Runtime.Store.Flow.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if f == nil {
		return response.NotFound(c, "flow not found")
	}

	doc := exportDocument{Gosint: kindFlow, Version: 1}
	d := document(*f)
	doc.Flow = &d

	return sendJSON(c, fileName(f.Name, "flow")+".flow.json", doc)
}

// ExportAll answers every flow of the caller as one file.
func (h Handler) ExportAll(c fiber.Ctx) error {
	flows, err := h.Runtime.Store.Flow.List(c, access.User(c).ID, "")
	if err != nil {
		return response.Internal(c, err)
	}

	doc := exportDocument{Gosint: kindFlows, Version: 1, Flows: []flowDocument{}}
	for _, f := range flows {
		doc.Flows = append(doc.Flows, document(f))
	}

	return sendJSON(c, "flows.json", doc)
}

func sendJSON(c fiber.Ctx, name string, v any) error {
	data, err := response.Marshal(v)
	if err != nil {
		return response.Internal(c, err)
	}

	c.Attachment(name)
	c.Set(fiber.HeaderContentType, "application/json")

	return c.Send(data)
}

// Import reads a flow file (one flow or several) as the multipart field
// "file" and makes the caller's own flows of it.
func (h Handler) Import(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return response.BadRequest(c, "send the file as the multipart field named file")
	}

	f, err := fh.Open()
	if err != nil {
		return response.Internal(c, err)
	}

	defer f.Close() //nolint:errcheck // read-only

	data, err := io.ReadAll(io.LimitReader(f, maxFlowFile))
	if err != nil {
		return response.Internal(c, err)
	}

	var doc exportDocument
	if err := response.Unmarshal(data, &doc); err != nil || (doc.Flow == nil && doc.Flows == nil) {
		return response.BadRequest(c, "the file is not a flow export")
	}

	docs := doc.Flows
	if doc.Flow != nil {
		docs = append(docs, *doc.Flow)
	}

	if len(docs) == 0 {
		return response.BadRequest(c, "the file holds no flow")
	}

	out := importResponse{Flows: []flowmodel.Flow{}, Skipped: []string{}}
	owner, now := access.User(c).ID, time.Now().UTC()
	for i, d := range docs {
		name := d.Name
		if strings.TrimSpace(name) == "" {
			name = fmt.Sprintf("flow %d", i+1)
		}

		if err := validation.NormalizeAndValidate(&d); err != nil {
			out.Skipped = append(out.Skipped, name+": "+validation.Summary(validation.Humanize(err)))

			continue
		}

		if err := coreflow.Check(d.Definition, h.Runtime.Transforms); err != nil {
			out.Skipped = append(out.Skipped, name+": "+err.Error())

			continue
		}

		fl := &flowmodel.Flow{ID: ids.New(), OwnerID: owner, Name: d.Name, Description: d.Description,
			Definition: d.Definition, CreatedAt: now, UpdatedAt: now}
		err := h.Runtime.Store.Flow.Create(c, fl)
		if database.UniqueViolation(err) {
			out.Skipped = append(out.Skipped, name+": you already have a flow of that name")

			continue
		}

		if err != nil {
			return response.Internal(c, err)
		}

		out.Imported++
		out.Flows = append(out.Flows, *fl)
	}

	if out.Imported == 0 && len(out.Skipped) > 0 {
		return response.BadRequest(c, "nothing imported: "+strings.Join(out.Skipped, ", "))
	}

	return response.Success(c, out)
}
