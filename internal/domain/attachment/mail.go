// Gosint, Copyright (c) 2026 YouSysAdmin

package attachment

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/mailparse"
	"github.com/yousysadmin/gosint/internal/core/response"
	amodel "github.com/yousysadmin/gosint/internal/models/attachment"
)

// An attached .eml opens as a message: the parsed shape for the viewer,
// and each part on its own for a download. The bytes stay the record,
// a parse happens on every open and is bounded by the file size cap.

// mailType is the content type of an .eml file.
const mailType = "message/rfc822"

// isMail says the file is a message, by its type or, for a file
// uploaded before the sniffer knew .eml, by its name.
func isMail(a *amodel.Attachment) bool {
	return a.ContentType == mailType || strings.EqualFold(filepath.Ext(a.Name), ".eml")
}

// readMail loads and parses the file. A refusal is answered already.
func (h Handler) readMail(c fiber.Ctx) (*amodel.Attachment, *mailparse.Email, error, bool) {
	a, err := h.Runtime.Store.Attachment.Get(c, access.Sketch(c).ID, c.Params("file_id"))
	if err != nil {
		return nil, nil, response.Internal(c, err), false
	}

	if a == nil {
		return nil, nil, response.NotFound(c, "file not found"), false
	}

	if !isMail(a) {
		return nil, nil, response.BadRequest(c, "the file is not an email message"), false
	}

	r, _, err := h.Runtime.Blobs.Open(c, a.Hash, nil)
	if errors.Is(err, blob.ErrNotFound) {
		h.Runtime.Log.Error("file has a record but no bytes", "file", a.ID, "hash", a.Hash)

		return nil, nil, response.NotFound(c, "the file's bytes are missing"), false
	}

	if err != nil {
		return nil, nil, response.Internal(c, err), false
	}

	defer r.Close() //nolint:errcheck // read-only

	limit := h.Runtime.FileLimits.Get().MaxSize
	raw, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, nil, response.Internal(c, err), false
	}

	if int64(len(raw)) > limit {
		return nil, nil, response.Coded(c, fiber.StatusRequestEntityTooLarge,
			fmt.Sprintf("the file is over the %d MB limit", limit>>20)), false
	}

	msg, err := mailparse.Parse(raw)
	if err != nil {
		return nil, nil, response.Coded(c, fiber.StatusUnprocessableEntity, "the file could not be read as an email"), false
	}

	return a, msg, nil, true
}

// Mail answers the parsed message. Parts travel as references, each
// downloaded on its own, so a message with a big attachment opens as
// fast as a small one.
func (h Handler) Mail(c fiber.Ctx) error {
	a, msg, resp, ok := h.readMail(c)
	if !ok {
		return resp
	}

	etag := `"` + a.Hash + `-mail"`
	if c.Get(fiber.HeaderIfNoneMatch) == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}

	out := Mail{
		From: msg.From, FromName: msg.FromName, To: msg.To, Cc: msg.Cc, Date: msg.Date, Subject: msg.Subject,
		MessageID: msg.MessageID, Received: msg.Received, Headers: make([]MailHeader, 0, len(msg.HeaderList)),
		Text: msg.TextBody, HTML: msg.HTMLBody, Attachments: make([]MailPart, 0, len(msg.Attachments)),
	}
	if out.To == nil {
		out.To = []string{}
	}

	if out.Cc == nil {
		out.Cc = []string{}
	}

	if out.Received == nil {
		out.Received = []string{}
	}

	for _, hd := range msg.HeaderList {
		out.Headers = append(out.Headers, MailHeader{Name: hd.Name, Value: hd.Value})
	}

	base := URLOf(a.SketchID, a.ID) + "/mail/parts/"
	for i, p := range msg.Attachments {
		out.Attachments = append(out.Attachments, MailPart{Index: i, Name: p.Filename, ContentType: p.ContentType,
			ContentID: p.ContentID, Size: p.Size, URL: base + strconv.Itoa(i)})
	}

	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")

	return response.Success(c, mailResponse{Mail: out})
}

// MailPart sends one attachment of the message. Its type is read from
// its name and bytes like an uploaded file's, never the one the message
// declares, which its sender chose, and decides by the same rule
// whether it may render inline.
func (h Handler) MailPart(c fiber.Ctx) error {
	_, msg, resp, ok := h.readMail(c)
	if !ok {
		return resp
	}

	i, err := strconv.Atoi(c.Params("index"))
	if err != nil || i < 0 || i >= len(msg.Attachments) {
		return response.NotFound(c, "no such part")
	}

	p := msg.Attachments[i]
	name := p.Filename
	if name == "" {
		name = "part-" + strconv.Itoa(i)
	}

	ct := Sniff(name, p.Content[:min(len(p.Content), sniffLen)])
	disposition := "attachment"
	if inlineSafe(ct) && c.Query("download") == "" {
		disposition = "inline"
		c.Set(fiber.HeaderXFrameOptions, "SAMEORIGIN")
	}

	c.Set(fiber.HeaderContentType, ct)
	guardFile(c, ct)
	c.Set(fiber.HeaderCacheControl, "private, max-age=3600")
	c.Set(fiber.HeaderContentDisposition, mime.FormatMediaType(disposition, map[string]string{"filename": name}))

	return c.Send(p.Content)
}
