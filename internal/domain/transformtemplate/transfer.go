// Gosint, Copyright (c) 2026 YouSysAdmin

package transformtemplate

import (
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/template"
	"github.com/yousysadmin/gosint/internal/core/types"
	"github.com/yousysadmin/gosint/internal/database"
	tplmodel "github.com/yousysadmin/gosint/internal/models/transformtemplate"
)

// A template IS its YAML, so it moves as a .yaml file, and several
// move as one file with "---" between them. The import makes the
// caller's own private templates, skipping one that does not parse,
// names a built-in transform or a template the caller already has.

const (
	maxTemplateFile = 4 << 20
	separator       = "\n---\n"
)

// importResponse says what an import did.
type importResponse struct {
	Imported  int                 `json:"imported"`
	Templates []tplmodel.Template `json:"templates"`
	Skipped   []string            `json:"skipped"`
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func sendYAML(c fiber.Ctx, name, body string) error {
	c.Attachment(name)
	c.Set(fiber.HeaderContentType, "application/yaml")

	return c.SendString(body)
}

// Export answers one template the caller may see as a YAML file.
func (h Handler) Export(c fiber.Ctx) error {
	t, err := h.Runtime.Store.Template.Get(c, access.User(c).ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if t == nil {
		return response.NotFound(c, "template not found")
	}

	name := strings.Trim(unsafeName.ReplaceAllString(t.Name, "_"), "_. ")
	if name == "" {
		name = "template"
	}

	return sendYAML(c, name+".yaml", strings.TrimSpace(t.Source)+"\n")
}

// ExportAll answers every template the caller owns as one YAML file.
func (h Handler) ExportAll(c fiber.Ctx) error {
	all, err := h.Runtime.Store.Template.Visible(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	var parts []string
	for _, t := range all {
		if t.OwnerID == access.User(c).ID {
			parts = append(parts, strings.TrimSpace(t.Source))
		}
	}

	return sendYAML(c, "transforms.yaml", strings.Join(parts, separator)+"\n")
}

// Import reads one or several templates from the multipart field
// "file" and makes the caller's own private templates of them.
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

	data, err := io.ReadAll(io.LimitReader(f, maxTemplateFile))
	if err != nil {
		return response.Internal(c, err)
	}

	out := importResponse{Templates: []tplmodel.Template{}, Skipped: []string{}}
	owner, now := access.User(c).ID, time.Now().UTC()
	docs := 0
	for i, src := range splitDocuments(string(data)) {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}

		docs++
		parsed, err := template.Parse([]byte(src), types.Builtin())
		if err != nil {
			out.Skipped = append(out.Skipped, "document "+strconv.Itoa(i+1)+": "+err.Error())

			continue
		}

		if _, builtin := h.Runtime.Transforms.Get(parsed.Name); builtin {
			out.Skipped = append(out.Skipped, parsed.Name+": the name of a built-in transform")

			continue
		}

		t := &tplmodel.Template{ID: ids.New(), OwnerID: owner, Name: parsed.Name, Description: parsed.Description,
			Category: parsed.Category, InputType: parsed.Input.Type, OutputType: parsed.Output.Type, Source: src + "\n",
			CreatedAt: now, UpdatedAt: now}
		err = h.Runtime.Store.Template.Create(c, t)
		if database.UniqueViolation(err) {
			out.Skipped = append(out.Skipped, parsed.Name+": you already have a template of that name")

			continue
		}

		if err != nil {
			return response.Internal(c, err)
		}

		out.Imported++
		out.Templates = append(out.Templates, *t)
	}

	if docs == 0 {
		return response.BadRequest(c, "the file holds no template")
	}

	if out.Imported == 0 {
		return response.BadRequest(c, "nothing imported: "+strings.Join(out.Skipped, ", "))
	}

	return response.Success(c, out)
}

// documentMarker is a line starting a YAML document: three dashes at the
// start of a line, then nothing but spaces or a comment, a CRLF file
// included. A dash line inside a block scalar is indented, so it is not
// one.
var documentMarker = regexp.MustCompile(`(?m)^---[ \t]*(?:#[^\n]*)?\r?$`)

// splitDocuments cuts a multi-document YAML file into its documents.
func splitDocuments(data string) []string {
	return documentMarker.Split(data, -1)
}
