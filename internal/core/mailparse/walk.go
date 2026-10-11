// Gosint, Copyright (c) 2026 YouSysAdmin

package mailparse

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// Structural limits on the MIME tree.
//
// These exist because a message comes from anyone, and because Parse
// takes no context: once it starts there is nothing to cancel it with.
// The file size cap upstream bounds the BYTES but not the SHAPE, and the
// shape is what costs.
//
// A 3.6 MB message nesting multipart/mixed 50000 levels deep held a
// goroutine for over two minutes in testing - the size limit let it
// straight through, and a handful of them would take the listener down.
// Real mail nests three or four levels (mixed > alternative > related)
// and carries a few dozen parts, so both ceilings sit far above anything
// legitimate and far below anything ruinous.
const (
	maxMIMEDepth = 20
	maxMIMEParts = 500
)

// ErrTooComplex marks a message whose MIME tree exceeds those limits.
// Callers treat it like any other parse failure: the bytes are kept,
// nothing is trusted.
var ErrTooComplex = errors.New("mime tree is too deeply nested or has too many parts")

// collector walks a MIME tree and gathers what a message actually says.
//
// A STRUCT rather than a function taking the destination and two
// counters: the remaining budget is state that spans the whole walk, and
// as parameters it had to be threaded through every recursive call and
// one of the two passed by pointer so siblings could see each other's
// spending. Here the recursion carries only what genuinely varies with
// depth, which is the depth.
type collector struct {
	into *Email

	// Parts left before the tree counts as hostile. Shared across the
	// whole walk, so a tree that is wide rather than deep is bounded
	// too.
	partsLeft int
}

// gather reads one multipart container and everything under it.
func (c *collector) gather(r io.Reader, boundary string, depth int) error {
	if depth > maxMIMEDepth {
		return ErrTooComplex
	}

	parts := multipart.NewReader(r, boundary)
	for {
		part, err := parts.NextPart()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return fmt.Errorf("multipart read: %w", err)
		}

		if c.partsLeft--; c.partsLeft < 0 {
			_ = part.Close()

			return ErrTooComplex
		}

		err = c.take(part, depth)
		_ = part.Close()

		if err != nil {
			return err
		}
	}
}

// take handles a single part: descend into it, keep it as an attachment,
// or read it as one of the two bodies.
func (c *collector) take(part *multipart.Part, depth int) error {
	media, params := mediaType(part.Header.Get("Content-Type"), "application/octet-stream")
	disposition, dispParams, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
	encoding := part.Header.Get("Content-Transfer-Encoding")
	contentID := strings.Trim(strings.TrimSpace(part.Header.Get("Content-ID")), "<>")

	if strings.HasPrefix(media, "multipart/") {
		// A container with no boundary describes nothing, so there is
		// nothing under it to read. Skipped rather than refused: the
		// rest of the message is still worth having.
		if nested := params["boundary"]; nested != "" {
			return c.gather(part, nested, depth+1)
		}

		return nil
	}

	// A non-text part with a Content-ID is an embedded image even when
	// it carries no filename, and reading it as a body would lose it.
	name := attachmentName(disposition, dispParams, params)
	if name.isAttachment || (contentID != "" && !strings.HasPrefix(media, "text/")) {
		return c.keep(part, media, encoding, name.filename, contentID)
	}

	body, err := readText(part, encoding, params["charset"])
	if err != nil {
		return err
	}

	c.into.addBody(media, body)

	return nil
}

// keep decodes a part and files it as an attachment.
func (c *collector) keep(part *multipart.Part, media, encoding, filename, contentID string) error {
	raw, err := io.ReadAll(part)
	if err != nil {
		return fmt.Errorf("read attachment: %w", err)
	}

	content, err := unwrapTransfer(raw, encoding)
	if err != nil {
		return fmt.Errorf("decode attachment: %w", err)
	}

	c.into.Attachments = append(c.into.Attachments, Attachment{
		Filename: filename,
		// The bare type, without the parameters that came with it: a
		// charset or a name belongs to the part, not to the file.
		ContentType: media,
		ContentID:   contentID,
		Content:     content,
		Size:        int64(len(content)),
	})

	return nil
}

// naming is what a part says about being a file.
type naming struct {
	isAttachment bool
	filename     string
}

// attachmentName decides whether a part is a file rather than a body.
//
// THREE signals, any of which is enough. A well-formed message says
// `Content-Disposition: attachment`. A great many say `inline` and give
// a filename anyway, which is how an embedded image arrives. Older
// senders skip the disposition entirely and put `name` on the content
// type. Requiring the first would drop attachments from all three.
func attachmentName(disposition string, dispParams, typeParams map[string]string) naming {
	filename := cmp.Or(dispParams["filename"], typeParams["name"])

	return naming{
		isAttachment: disposition == "attachment" || filename != "",
		filename:     headerText(filename),
	}
}

// readText reads a part as a decoded string.
func readText(r io.Reader, encoding, declaredCharset string) (string, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	decoded, err := unwrapTransfer(raw, encoding)
	if err != nil {
		return "", err
	}

	return asText(decoded, declaredCharset), nil
}

// mediaType parses a Content-Type, falling back when it is absent or
// malformed. The fallback differs by caller, which is why it is an
// argument: a message with no content type is plain text by RFC 2045,
// but a PART that declares something unparseable is safer treated as
// opaque bytes than as text.
func mediaType(header, fallback string) (string, map[string]string) {
	if header == "" {
		return contentTypeTextPlain, map[string]string{}
	}

	media, params, err := mime.ParseMediaType(header)
	if err != nil {
		return fallback, map[string]string{}
	}

	return media, params
}
