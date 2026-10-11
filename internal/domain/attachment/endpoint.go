// Gosint, Copyright (c) 2026 YouSysAdmin

// Package attachment is files attached to entities: a screenshot of a
// message, a document, an image of a person. The record is in the
// store, the bytes in the blob store under their sha256, and both are
// served through the server behind the sketch's access.
package attachment

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/filemeta"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	amodel "github.com/yousysadmin/gosint/internal/models/attachment"
	graphmodel "github.com/yousysadmin/gosint/internal/models/graph"
)

// Handler serves the file routes of a sketch.
type Handler struct {
	Runtime *env.Runtime
}

// URLOf is where a file is served.
func URLOf(sketchID, fileID string) string {
	return "/api/sketches/" + sketchID + "/files/" + fileID
}

// view is the wire form of a record.
func view(a amodel.Attachment) File {
	return File{Attachment: a, URL: URLOf(a.SketchID, a.ID), IsImage: a.IsImage()}
}

// sniffLen is what http.DetectContentType looks at.
const sniffLen = 512

// Upload attaches the multipart "file" to a node of the sketch. The
// type is decided from the name, then from the first bytes, never from
// the client's header.
func (h Handler) Upload(c fiber.Ctx) error {
	sketchID, nodeID := access.Sketch(c).ID, c.Params("node_id")
	n, err := h.Runtime.Store.Graph.Node(c, sketchID, nodeID)
	if err != nil {
		return response.Internal(c, err)
	}

	if n == nil {
		return response.NotFound(c, "node not found")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return response.BadRequest(c, "send the file as the multipart field named file")
	}

	// The name as sent: a browser sends a plain one, anything with a
	// path in it is not a browser.
	name := fh.Filename
	if err := blob.ValidName(name); err != nil {
		return response.BadRequest(c, "the file name is not allowed, use a plain name without slashes")
	}

	f, err := fh.Open()
	if err != nil {
		return response.Internal(c, err)
	}

	defer f.Close() //nolint:errcheck // read-only

	// The whole file is read once: the bytes go to the store, and what
	// they say about themselves (EXIF, a PDF's info, an Office
	// document's properties) goes on the record and the entity.
	data, err := io.ReadAll(io.LimitReader(f, h.Runtime.FileLimits.Get().MaxSize+1))
	if err != nil {
		return response.Internal(c, err)
	}

	head := data[:min(len(data), sniffLen)]
	hash, size, err := h.Runtime.Blobs.Put(c, bytes.NewReader(data), h.Runtime.FileLimits.Get().MaxSize)
	if errors.Is(err, blob.ErrTooLarge) {
		return response.Coded(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("the file is over the %d MB limit", h.Runtime.FileLimits.Get().MaxSize>>20))
	}

	if err != nil {
		return response.Internal(c, err)
	}

	a := &amodel.Attachment{ID: ids.New(), SketchID: sketchID, NodeID: nodeID, Name: name, ContentType: Sniff(name, head),
		Size: size, Hash: hash, UploaderID: access.User(c).ID, CreatedAt: time.Now().UTC()}
	a.Metadata = filemeta.Extract(name, a.ContentType, data)
	if err := h.Runtime.Store.Attachment.Create(c, a); err != nil {
		// Bytes first, then the record: on a failed record the bytes are
		// released unless another record already names them.
		Release(context.WithoutCancel(c), h.Runtime, []string{hash})

		return response.Internal(c, err)
	}

	if err := KeepBytes(c, h.Runtime, hash, data); err != nil {
		return response.Internal(c, err)
	}

	if patch := MetaPatch(n.Type, n.Properties, a.Metadata); len(patch) > 0 {
		if _, err := h.Runtime.Store.Graph.UpdateNode(c, sketchID, nodeID, graphmodel.NodePatch{Properties: patch}); err != nil {
			h.Runtime.Log.Warn("files: filling the entity from the file failed", "node", nodeID, "err", err)
		}
	}

	return response.Created(c, fileResponse{File: view(*a)})
}

// KeepBytes makes sure the blob a new record names is there. Put finds
// an existing blob and keeps it, so a sweep that counted no record for
// it just before the record landed may have deleted it since; it is
// written again from the bytes in hand.
func KeepBytes(ctx context.Context, rt *env.Runtime, hash string, data []byte) error {
	ok, err := rt.Blobs.Exists(ctx, hash)
	if err != nil || ok {
		return err
	}

	_, _, err = rt.Blobs.Put(ctx, bytes.NewReader(data), int64(len(data))+1)

	return err
}

// metaFields says which of an entity's fields a file's metadata fills,
// when the field is still empty: a photo takes the camera, the moment
// and the place, a document its title, author, dates and keywords, a
// file its dates.
var metaFields = map[string]map[string]string{
	"photo":    {"taken_at": filemeta.TakenAt, "latitude": filemeta.Latitude, "longitude": filemeta.Longitude},
	"document": {"author": filemeta.Author, "subject": filemeta.Subject, "created_date": filemeta.CreatedAt, "modified_date": filemeta.ModifiedAt},
	"file":     {"created_date": filemeta.CreatedAt, "modified_date": filemeta.ModifiedAt},
}

// MetaPatch is what the file's metadata fills on an entity of typ whose
// properties are props: the empty fields only, the person's own stay.
func MetaPatch(typ string, props map[string]any, meta map[string]any) map[string]any {
	patch := map[string]any{}
	for field, key := range metaFields[typ] {
		v, ok := meta[key]
		if !ok || v == nil || v == "" {
			continue
		}

		if cur, set := props[field]; set && cur != nil && cur != "" {
			continue
		}

		patch[field] = v
	}

	if typ == "photo" {
		if camera := strings.TrimSpace(strings.TrimSpace(str(meta, filemeta.CameraMake)) + " " + str(meta, filemeta.CameraModel)); camera != "" && str(props, "camera") == "" {
			patch["camera"] = camera
		}
	}

	if typ == "document" {
		if kw := str(meta, filemeta.Keywords); kw != "" {
			if cur, _ := props["keywords"].([]any); len(cur) == 0 {
				list := []any{}
				for _, k := range strings.FieldsFunc(kw, func(r rune) bool { return r == ',' || r == ';' }) {
					if k = strings.TrimSpace(k); k != "" {
						list = append(list, k)
					}
				}

				patch["keywords"] = list
			}
		}
	}

	return patch
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)

	return s
}

// knownTypes are extensions the OS mime table may not know and the
// bytes do not tell apart (an .eml reads as plain text).
var knownTypes = map[string]string{".eml": mailType}

// Sniff names a file's type: the extension first, then the bytes, never
// what a client said. The parameters a mime table may add are dropped.
func Sniff(name string, head []byte) string {
	ext := strings.ToLower(filepath.Ext(name))
	ct := knownTypes[ext]
	if ct == "" {
		ct = mime.TypeByExtension(ext)
	}

	if ct == "" {
		ct = http.DetectContentType(head)
	}

	ct, _, _ = strings.Cut(ct, ";")

	return strings.ToLower(strings.TrimSpace(ct))
}

// List answers a node's files.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Attachment.ListForNode(c, access.Sketch(c).ID, c.Params("node_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	out := listResponse{Files: make([]File, 0, len(list))}
	for _, a := range list {
		out.Files = append(out.Files, view(a))
	}

	return response.Success(c, out)
}

// inlineTypes is what a browser may render on this origin, each type
// one the browser draws as media or text, never as a document. Every
// other type downloads: html, svg or an xml flavour is user content,
// and inline it could run as the site.
var inlineTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/avif": true, "image/bmp": true,
	"audio/mpeg": true, "audio/ogg": true, "audio/wav": true, "audio/webm": true, "audio/aac": true, "audio/flac": true,
	"video/mp4": true, "video/webm": true, "video/ogg": true, "video/quicktime": true,
	"application/pdf": true, "text/plain": true,
}

// inlineSafe reports whether ct is in inlineTypes, read the way a
// browser reads it, case folded and without parameters.
func inlineSafe(ct string) bool {
	parsed, _, err := mime.ParseMediaType(ct)

	return err == nil && inlineTypes[strings.ToLower(parsed)]
}

// fileSandbox is the policy of a file served from this origin: nothing
// it holds may run, load or navigate. A PDF goes without, the browser's
// viewer does not render inside a sandbox, and its type is never one a
// script runs in.
const fileSandbox = "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; sandbox"

// guardFile sets the headers every served file carries.
func guardFile(c fiber.Ctx, contentType string) {
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("Cross-Origin-Resource-Policy", "same-origin")
	if parsed, _, err := mime.ParseMediaType(contentType); err != nil || strings.ToLower(parsed) != "application/pdf" {
		c.Set(fiber.HeaderContentSecurityPolicy, fileSandbox)
	}
}

// Serve sends the bytes. ?download=1 forces a download, a Range is
// honoured for one range.
func (h Handler) Serve(c fiber.Ctx) error {
	a, err := h.Runtime.Store.Attachment.Get(c, access.Sketch(c).ID, c.Params("file_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if a == nil {
		return response.NotFound(c, "file not found")
	}

	etag := `"` + a.Hash + `"`
	if c.Get(fiber.HeaderIfNoneMatch) == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	disposition := "attachment"
	if inlineSafe(a.ContentType) && c.Query("download") == "" {
		disposition = "inline"
		// The console shows a PDF in a frame of its own, which the
		// DENY every other response carries would refuse.
		c.Set(fiber.HeaderXFrameOptions, "SAMEORIGIN")
	}

	rng := parseRange(c.Get(fiber.HeaderRange), a.Size)
	r, obj, err := h.Runtime.Blobs.Open(c, a.Hash, rng)
	if errors.Is(err, blob.ErrRange) {
		c.Set(fiber.HeaderContentRange, "bytes */"+strconv.FormatInt(a.Size, 10))

		return c.SendStatus(fiber.StatusRequestedRangeNotSatisfiable)
	}

	if errors.Is(err, blob.ErrNotFound) {
		h.Runtime.Log.Error("file has a record but no bytes", "file", a.ID, "hash", a.Hash)

		return response.NotFound(c, "the file's bytes are missing")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderAcceptRanges, "bytes")
	c.Set(fiber.HeaderContentType, a.ContentType)
	guardFile(c, a.ContentType)
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	c.Set(fiber.HeaderContentDisposition, mime.FormatMediaType(disposition, map[string]string{"filename": a.Name}))
	if rng != nil {
		c.Set(fiber.HeaderContentRange, fmt.Sprintf("bytes %d-%d/%d", obj.Offset, obj.Offset+obj.Length-1, obj.Size))
		c.Status(fiber.StatusPartialContent)
	}

	return c.SendStream(r, int(obj.Length))
}

// parseRange reads a Range header of one range, a suffix range
// counted from size. Nil serves the whole file: a header that does
// not parse, or asks for several ranges, is ignored as RFC 9110 says.
// A start past the end is the store's ErrRange, answered 416.
func parseRange(h string, size int64) *blob.Range {
	spec, ok := strings.CutPrefix(h, "bytes=")
	if !ok || strings.Contains(spec, ",") {
		return nil
	}

	startS, endS, ok := strings.Cut(strings.TrimSpace(spec), "-")
	if !ok {
		return nil
	}

	if startS == "" {
		n, err := strconv.ParseInt(endS, 10, 64)
		if err != nil || n <= 0 || size <= 0 {
			return nil
		}

		return &blob.Range{Start: max(0, size-n), End: -1}
	}

	start, err := strconv.ParseInt(startS, 10, 64)
	if err != nil || start < 0 {
		return nil
	}

	rng := &blob.Range{Start: start, End: -1}
	if endS != "" {
		end, err := strconv.ParseInt(endS, 10, 64)
		if err != nil || end < start {
			return nil
		}

		rng.End = end
	}

	return rng
}

// Delete removes a file and its bytes when nothing else names them.
func (h Handler) Delete(c fiber.Ctx) error {
	sketchID := access.Sketch(c).ID
	a, err := h.Runtime.Store.Attachment.Get(c, sketchID, c.Params("file_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if a == nil {
		return response.NotFound(c, "file not found")
	}

	if _, err := h.Runtime.Store.Attachment.Delete(c, sketchID, a.ID); err != nil {
		return response.Internal(c, err)
	}

	Release(c, h.Runtime, []string{a.Hash})

	return response.NoContent(c)
}

// Release lets go of every blob of hashes that no record names any
// more: the bytes stay for files.retain_deleted, so an undo brings the
// file back, and the sweep deletes them after. Called after the records
// went, with hashes read before. A failure is logged, not returned.
func Release(ctx context.Context, rt *env.Runtime, hashes []string) {
	var free []string
	for _, hash := range hashes {
		n, err := rt.Store.Attachment.CountByHash(ctx, hash)
		if err != nil {
			rt.Log.Warn("files: counting references failed, the blob stays", "hash", hash, "err", err)

			continue
		}

		if n == 0 {
			free = append(free, hash)
		}
	}

	if err := rt.Store.Attachment.MarkReleased(ctx, free, time.Now().UTC()); err != nil {
		rt.Log.Warn("files: marking released blobs failed, the sweep will not find them", "err", err)
	}
}

// Sweep deletes the blobs let go before the moment, unless a record
// names them again by now, and forgets the marks. The maintenance job,
// safe to run on every node at once: a blob already gone is not an
// error.
func Sweep(ctx context.Context, rt *env.Runtime, before time.Time) (deleted int, err error) {
	keys, err := rt.Store.Attachment.ReleasedBefore(ctx, before)
	if err != nil {
		return 0, err
	}

	for _, key := range keys {
		n, err := rt.Store.Attachment.CountByKey(ctx, key)
		if err != nil {
			return deleted, err
		}

		if n == 0 {
			if err := deleteBlob(ctx, rt, key); err != nil && !errors.Is(err, blob.ErrNotFound) {
				return deleted, fmt.Errorf("delete blob %s: %w", key, err)
			}

			deleted++
		}

		if err := rt.Store.Attachment.Unmark(ctx, key); err != nil {
			return deleted, err
		}
	}

	return deleted, nil
}

// storedDeleter deletes a blob by the name it is stored under.
type storedDeleter interface {
	DeleteStored(ctx context.Context, name string) error
}

// deleteBlob deletes the blob a mark names: the keyed hash a record
// holds is the name the sealed bytes are stored under.
func deleteBlob(ctx context.Context, rt *env.Runtime, key string) error {
	if d, ok := rt.Blobs.(storedDeleter); ok {
		return d.DeleteStored(ctx, key)
	}

	return rt.Blobs.Delete(ctx, key)
}
