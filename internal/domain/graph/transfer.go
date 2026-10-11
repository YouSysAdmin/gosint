// Gosint, Copyright (c) 2026 YouSysAdmin

package graph

import (
	"context"
	"encoding/base64"
	"encoding/json/jsontext"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/envelope"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	amodel "github.com/yousysadmin/gosint/internal/models/analysis"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
)

// Export documents. A sketch export is the sketch, its graph and its
// files, an investigation export is the investigation, every sketch
// as such a document, and the analyses. Either is sealed under a
// passphrase when one is given (core/envelope), so a file that leaves
// the server is only readable by whoever knows it.
const (
	kindSketch        = "sketch"
	kindInvestigation = "investigation"
	documentVersion   = 1
	maxDocument       = 256 << 20
)

// ImportBodyLimit is the request body an import of an export may send:
// the largest document plus room for the multipart framing. The server
// raises its body limit to it on the import routes.
const ImportBodyLimit = maxDocument + 1<<20

// sketchDocument is a sketch and its whole graph, for download.
func (h Handler) sketchDocument(c fiber.Ctx, sk *sketchmodel.Sketch) (sketchDocument, error) {
	doc := sketchDocument{Gosint: kindSketch, Version: documentVersion, Sketch: *sk}
	nodes, edges, err := h.Runtime.Store.Graph.Graph(c, sk.ID)
	if err != nil {
		return doc, err
	}

	doc.Nodes, doc.Edges = nodes, edges

	// The files travel inside the document, base64, so a sketch moves
	// between installs as one file.
	records, err := h.Runtime.Store.Attachment.ListForSketch(c, sk.ID)
	if err != nil {
		return doc, err
	}

	doc.Files = make([]exportFile, 0, len(records))
	for _, a := range records {
		// Cut to the limit the file would come back corrupt, so a file
		// over a limit lowered since its upload is left out instead.
		if a.Size > h.Runtime.FileLimits.Get().MaxSize {
			h.Runtime.Log.Warn("export: a file is over files.max_size, left out", "file", a.ID, "size", a.Size)

			continue
		}

		r, _, err := h.Runtime.Blobs.Open(c, a.Hash, nil)
		if err != nil {
			h.Runtime.Log.Warn("export: a file's bytes are missing, left out", "file", a.ID, "err", err)

			continue
		}

		data, err := io.ReadAll(io.LimitReader(r, h.Runtime.FileLimits.Get().MaxSize))
		_ = r.Close()
		if err != nil {
			return doc, err
		}

		doc.Files = append(doc.Files, exportFile{ID: a.ID, Node: a.NodeID, Name: a.Name, ContentType: a.ContentType,
			Data: base64.StdEncoding.EncodeToString(data)})
	}

	return doc, nil
}

// Export answers the sketch and its graph as one document, sealed
// when a passphrase is given.
func (h Handler) Export(c fiber.Ctx) error {
	in, resp, ok := validation.BindOptional[exportInput](c)
	if !ok {
		return resp
	}

	sk := access.Sketch(c)
	doc, err := h.sketchDocument(c, sk)
	if err != nil {
		return response.Internal(c, err)
	}

	return sendDocument(c, sk.Title, in.Passphrase, doc)
}

// ExportInvestigation answers the investigation, every sketch with its
// graph and files, and the analyses as one document.
func (h Handler) ExportInvestigation(c fiber.Ctx) error {
	in, resp, ok := validation.BindOptional[exportInput](c)
	if !ok {
		return resp
	}

	inv := access.Investigation(c).Investigation
	doc := investigationDocument{Gosint: kindInvestigation, Version: documentVersion, Investigation: *inv}
	sketches, err := h.Runtime.Store.Sketch.ListByInvestigation(c, inv.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	doc.Sketches = make([]sketchDocument, 0, len(sketches))
	for i := range sketches {
		sd, err := h.sketchDocument(c, &sketches[i])
		if err != nil {
			return response.Internal(c, err)
		}

		doc.Sketches = append(doc.Sketches, sd)
	}

	// The list leaves the content out, each analysis is read whole.
	list, err := h.Runtime.Store.Analysis.List(c, inv.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	doc.Analyses = make([]analysisDocument, 0, len(list))
	for _, a := range list {
		full, err := h.Runtime.Store.Analysis.Get(c, inv.ID, a.ID)
		if err != nil {
			return response.Internal(c, err)
		}

		if full != nil {
			doc.Analyses = append(doc.Analyses, analysisDocument{Title: full.Title, Content: full.Content})
		}
	}

	return sendDocument(c, inv.Name, in.Passphrase, doc)
}

// sendDocument marshals the document, seals it under the passphrase
// when one is given, and sends it as a download named after the title.
func sendDocument(c fiber.Ctx, title, passphrase string, doc any) error {
	data, err := response.Marshal(doc)
	if err != nil {
		return response.Internal(c, err)
	}

	ext := ".json"
	if strings.TrimSpace(passphrase) != "" {
		if data, err = envelope.Seal(passphrase, data); err != nil {
			return documentError(c, err)
		}

		ext = ".gosint"
	}

	c.Attachment(fileName(title) + ext)
	c.Set(fiber.HeaderContentType, "application/json")

	return c.Send(data)
}

// unsafeName is what a file name must not carry.
var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// fileName makes a plain file name of a title.
func fileName(title string) string {
	name := strings.Trim(unsafeName.ReplaceAllString(title, "_"), "_. ")
	if name == "" {
		return "export"
	}

	return name
}

// importSlots bounds the imports running at once: each holds the whole
// file, its decoded document and every entity it writes.
var importSlots = make(chan struct{}, 2)

// importSlot takes an import slot, false when every slot is busy.
func importSlot() (func(), bool) {
	select {
	case importSlots <- struct{}{}:
		return func() { <-importSlots }, true
	default:
		return nil, false
	}
}

// documentError answers a failure to read or seal an export: busy key
// derivation is the server's, anything else the file's.
func documentError(c fiber.Ctx, err error) error {
	if errors.Is(err, envelope.ErrBusy) {
		return response.Unavailable(c, err.Error())
	}

	return response.BadRequest(c, err.Error())
}

// readDocument reads the multipart file of an import, opening it with
// the passphrase field when it is sealed, and answers the file's name.
func readDocument(c fiber.Ctx) ([]byte, string, error) {
	// Parsed here under the import's own limit, since the app-wide body
	// limit would refuse an export carrying its files. The form is kept
	// by the request, so FormFile and FormValue read it back.
	if _, err := c.RequestCtx().MultipartFormWithLimit(ImportBodyLimit); err != nil {
		return nil, "", errors.New("send the export as the multipart field named file")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return nil, "", errors.New("send the export as the multipart field named file")
	}

	f, err := fh.Open()
	if err != nil {
		return nil, "", err
	}

	defer f.Close() //nolint:errcheck // read-only

	data, err := io.ReadAll(io.LimitReader(f, maxDocument))
	if err != nil {
		return nil, "", err
	}

	if envelope.IsSealed(data) {
		pass := c.FormValue("passphrase")
		if strings.TrimSpace(pass) == "" {
			return nil, "", errors.New("this export is encrypted, give its passphrase")
		}

		if data, err = envelope.Open(pass, data); err != nil {
			return nil, "", err
		}
	}

	name := strings.TrimSuffix(path.Base(fh.Filename), path.Ext(fh.Filename))

	return data, strings.TrimSpace(name), nil
}

// documentHead is what every export starts with.
type documentHead struct {
	Gosint string `json:"gosint"`
}

// kindOf tells a sketch export (marked, or a bare graph) from an
// investigation export.
func kindOf(data []byte) string {
	var head documentHead
	if response.Unmarshal(data, &head) == nil && head.Gosint == kindInvestigation {
		return kindInvestigation
	}

	return kindSketch
}

// importOf reads a sketch document as an import, checked as a body
// would be.
func importOf(data []byte) (importInput, error) {
	in := importInput{Source: "file"}
	if err := response.Unmarshal(data, &in); err != nil {
		return in, errors.New("the file is not a sketch export")
	}

	if err := validation.NormalizeAndValidate(&in); err != nil {
		return in, errors.New("the file is not a sketch export: " + validation.Summary(validation.Humanize(err)))
	}

	return in, nil
}

// ImportFile merges an export file into this sketch: its name is not
// read, the entities and relationships land in the sketch as they are.
func (h Handler) ImportFile(c fiber.Ctx) error {
	release, ok := importSlot()
	if !ok {
		return response.Unavailable(c, "other imports are running, try again shortly")
	}

	defer release()

	data, _, err := readDocument(c)
	if err != nil {
		return documentError(c, err)
	}

	if kindOf(data) == kindInvestigation {
		return response.BadRequest(c, "this is an investigation export, import it from the investigations page")
	}

	in, err := importOf(data)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	return h.runImport(c, in)
}

// ImportSketch makes a new sketch of an export file under the
// investigation, named as the file says, else after the file.
func (h Handler) ImportSketch(c fiber.Ctx) error {
	release, ok := importSlot()
	if !ok {
		return response.Unavailable(c, "other imports are running, try again shortly")
	}

	defer release()

	data, name, err := readDocument(c)
	if err != nil {
		return documentError(c, err)
	}

	if kindOf(data) == kindInvestigation {
		return response.BadRequest(c, "this is an investigation export, import it from the investigations page")
	}

	var doc sketchDocument
	_ = response.Unmarshal(data, &doc)
	in, err := importOf(data)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	sk, out, err := h.importSketch(c, access.Investigation(c).Investigation.ID, doc.Sketch, name, in)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, sketchImportResponse{Sketch: *sk, Result: out.out})
}

// importSketch creates the sketch and fills it from the import.
func (h Handler) importSketch(c fiber.Ctx, investigationID string, from sketchmodel.Sketch, fallback string,
	in importInput) (*sketchmodel.Sketch, imported, error) {
	now := time.Now().UTC()
	sk := &sketchmodel.Sketch{
		ID:              ids.New(),
		InvestigationID: investigationID,
		Title:           firstOf(from.Title, fallback, "Imported sketch"),
		Description:     from.Description,
		Status:          firstOf(from.Status, "active"),
		OwnerID:         access.User(c).ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := h.Runtime.Store.Sketch.Create(c, sk); err != nil {
		return nil, imported{}, err
	}

	out, err := h.doImport(c, sk.ID, in)
	if err != nil {
		return nil, imported{}, err
	}

	return sk, out, nil
}

// ImportInvestigation makes a new investigation of an export file, the
// caller its owner, with every sketch and the analyses. A sketch export
// becomes an investigation of one sketch.
func (h Handler) ImportInvestigation(c fiber.Ctx) error {
	release, ok := importSlot()
	if !ok {
		return response.Unavailable(c, "other imports are running, try again shortly")
	}

	defer release()

	data, name, err := readDocument(c)
	if err != nil {
		return documentError(c, err)
	}

	var doc investigationDocument
	if kindOf(data) == kindInvestigation {
		if err := response.Unmarshal(data, &doc); err != nil {
			return response.BadRequest(c, "the file is not an investigation export")
		}
	} else {
		var sd sketchDocument
		_ = response.Unmarshal(data, &sd)
		if _, err := importOf(data); err != nil {
			return response.BadRequest(c, err.Error())
		}

		doc.Investigation.Name = sd.Sketch.Title
		doc.Sketches = []sketchDocument{sd}
	}

	now := time.Now().UTC()
	inv := &invmodel.Investigation{
		ID:          ids.New(),
		Name:        firstOf(doc.Investigation.Name, name, "Imported investigation"),
		Description: doc.Investigation.Description,
		Status:      firstOf(doc.Investigation.Status, "active"),
		OwnerID:     access.User(c).ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.Runtime.Store.Investigation.Create(c, inv); err != nil {
		return response.Internal(c, err)
	}

	// A failure half way drops what was made, so the caller is not left
	// with part of an investigation.
	fail := func(err error) error {
		h.dropInvestigation(context.WithoutCancel(c), inv.ID)

		return response.Internal(c, err)
	}

	out := investigationImportResponse{Investigation: invmodel.WithRole{Investigation: *inv, Role: invmodel.RoleOwner}}

	// The ids a document carries are replaced in the analyses, whose
	// mentions name sketches and entities by id.
	renames := []string{}
	for _, sd := range doc.Sketches {
		data, err := response.Marshal(sd)
		if err != nil {
			return fail(err)
		}

		in, err := importOf(data)
		if err != nil {
			out.Errors = append(out.Errors, firstOf(sd.Sketch.Title, "a sketch")+": "+err.Error())

			continue
		}

		sk, res, err := h.importSketch(c, inv.ID, sd.Sketch, "", in)
		if err != nil {
			return fail(err)
		}

		out.Sketches = append(out.Sketches, *sk)
		out.NodesCreated += res.out.NodesCreated
		out.EdgesCreated += res.out.EdgesCreated
		out.FilesCreated += res.out.FilesCreated
		for _, e := range res.out.Errors {
			if len(out.Errors) < maxImportErrors {
				out.Errors = append(out.Errors, sk.Title+": "+e)
			}
		}

		if sd.Sketch.ID != "" {
			renames = append(renames, sd.Sketch.ID, sk.ID)
		}

		for old, id := range res.ids {
			renames = append(renames, old, id)
		}
	}

	replacer := strings.NewReplacer(renames...)
	for _, a := range doc.Analyses {
		content := jsontext.Value(replacer.Replace(string(a.Content)))
		if len(content) == 0 {
			content = jsontext.Value("{}")
		}

		if !content.IsValid() {
			out.Errors = append(out.Errors, "analysis "+a.Title+": the content is not JSON")

			continue
		}

		err := h.Runtime.Store.Analysis.Create(c, &amodel.Analysis{ID: ids.New(), InvestigationID: inv.ID,
			OwnerID: inv.OwnerID, Title: firstOf(a.Title, "Untitled"), Content: content, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			return fail(err)
		}

		out.AnalysesCreated++
	}

	return response.Created(c, out)
}

// dropInvestigation removes an investigation an import made, its
// sketches and records with it and their bytes released. A failure is
// logged, the import's own error is what the caller hears.
func (h Handler) dropInvestigation(ctx context.Context, id string) {
	sketches, err := h.Runtime.Store.Sketch.ListByInvestigation(ctx, id)
	if err != nil {
		h.Runtime.Log.Error("import: dropping a partial investigation failed", "investigation", id, "err", err)

		return
	}

	var hashes []string
	for _, sk := range sketches {
		more, err := h.Runtime.Store.Attachment.HashesOfSketch(ctx, sk.ID)
		if err != nil {
			h.Runtime.Log.Error("import: dropping a partial investigation failed", "investigation", id, "err", err)

			return
		}

		hashes = append(hashes, more...)
	}

	if err := h.Runtime.Store.Investigation.Delete(ctx, id); err != nil {
		h.Runtime.Log.Error("import: dropping a partial investigation failed", "investigation", id, "err", err)

		return
	}

	attachment.Release(ctx, h.Runtime, hashes)
}

// firstOf is the first non-blank string.
func firstOf(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}

	return ""
}

// relinkImages points the pictures of the written nodes at the files
// as they are now: an image URL names the file by id, which changed.
func (h Handler) relinkImages(c fiber.Ctx, sketchID string, written []graphmodel.Node, files map[string]string) {
	if len(files) == 0 {
		return
	}

	for _, n := range written {
		if n.Image == "" || !strings.Contains(n.Image, "/files/") {
			continue
		}

		old := path.Base(n.Image)
		id, ok := files[old]
		if !ok {
			continue
		}

		u := attachment.URLOf(sketchID, id)
		if _, err := h.Runtime.Store.Graph.UpdateNode(c, sketchID, n.ID, graphmodel.NodePatch{Image: &u}); err != nil {
			h.Runtime.Log.Warn("import: a picture was not relinked", "node", n.ID, "err", err)
		}
	}
}
