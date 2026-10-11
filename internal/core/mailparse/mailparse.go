// Gosint, Copyright (c) 2026 YouSysAdmin

// Package mailparse turns a raw RFC 5322 message into a normalized
// struct: decoded headers, a text body, an html body and attachments.
// Ported from mailyard, it reads the .eml files attached to entities.
//
// NOTHING HERE REFUSES A MESSAGE FOR BEING MALFORMED, and that is the
// governing decision. Mail on the internet is broken in every way it can
// be: absent dates, unparseable content types, charsets that lie,
// headers that are not UTF-8. A parser that returned an error for each
// of those would reject a large slice of real mail. So each of them has
// a documented fallback and the message goes through. The two things
// that DO fail are a message that is not a message at all, and a MIME
// tree built to exhaust the machine reading it, see walk.go.
package mailparse

import (
	"bytes"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const contentTypeTextPlain = "text/plain"

// Attachment is one file carried by a message, already decoded.
type Attachment struct {
	Filename    string
	ContentType string

	// ContentID is the part's Content-ID without brackets, what an HTML
	// body names in a cid: URL to embed the part as an image.
	ContentID string
	Content   []byte
	Size      int64
}

// Header is one header line as it stood in the message.
type Header struct {
	Name  string
	Value string
}

// Email is the normalized shape Parse produces.
//
// Headers keeps the first value of every header under its canonical
// name, decoded. HeaderList keeps every header IN MESSAGE ORDER, which
// is evidence in itself (the Received chain reads top down), and
// Received is that chain alone. The bytes stay the record, this struct
// is an interpretation.
type Email struct {
	MessageID string
	From      string

	// FromName is the display name on the From header, decoded, empty
	// when there is none.
	FromName string
	To       []string
	Cc       []string

	// ToHeader and CcHeader are the To and Cc headers with their
	// display names kept, re-encoded as one mailbox list each. Empty
	// when the header is absent or is not an address list.
	ToHeader string
	CcHeader string
	Subject  string
	Date     time.Time
	TextBody string
	HTMLBody string
	Headers  map[string]string

	// HeaderList is every header in message order, repeated ones kept.
	HeaderList []Header

	// Received is the Received chain, the nearest hop first.
	Received    []string
	Attachments []Attachment
}

// Parse reads a raw RFC 5322 message.
func Parse(raw []byte) (*Email, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("read message: %w", err)
	}

	out := &Email{Headers: make(map[string]string, len(msg.Header)), HeaderList: headerLines(raw)}
	for _, v := range msg.Header["Received"] {
		out.Received = append(out.Received, headerText(v))
	}

	for name, values := range msg.Header {
		// The first value only. A header repeated with different
		// contents is either a bug or an attack, and picking the first
		// is what every reader of this struct assumes.
		if len(values) > 0 {
			out.Headers[name] = headerText(values[0])
		}
	}

	out.describe(msg)

	media, params := mediaType(msg.Header.Get("Content-Type"), contentTypeTextPlain)
	if !strings.HasPrefix(media, "multipart/") {
		body, err := readText(msg.Body, msg.Header.Get("Content-Transfer-Encoding"), params["charset"])
		if err != nil {
			return nil, err
		}

		out.addBody(media, body)

		return out, nil
	}

	// The one malformation this does refuse. A multipart container with
	// no boundary has no readable content at all - there is no fallback
	// to choose, because nothing says where the parts begin.
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("multipart with no boundary")
	}

	walk := &collector{into: out, partsLeft: maxMIMEParts}
	if err := walk.gather(msg.Body, boundary, 1); err != nil {
		return nil, err
	}

	return out, nil
}

// describe fills in the fields that come from headers.
//
// Read from Headers where the value is plain text, since that map is
// already decoded, and from msg.Header where it has to be parsed as an
// address or a date - those want the value as it was sent.
func (e *Email) describe(msg *mail.Message) {
	// net/mail canonicalizes to Message-Id, so that is the only spelling
	// this map can hold. The angle brackets are RFC 5322 syntax rather
	// than part of the identifier, and every comparison downstream is
	// against a bare id.
	e.MessageID = strings.Trim(e.Headers["Message-Id"], "<>")
	e.Subject = e.Headers["Subject"]

	e.From = oneAddress(msg.Header.Get("From"))
	if addr, err := mail.ParseAddress(msg.Header.Get("From")); err == nil {
		e.FromName = addr.Name
	}

	e.To = addressList(msg.Header.Get("To"))
	e.Cc = addressList(msg.Header.Get("Cc"))
	e.ToHeader = mailboxList(msg.Header.Get("To"))
	e.CcHeader = mailboxList(msg.Header.Get("Cc"))

	// An absent or unreadable Date stays zero. Mailyard put the time of
	// reading there, but a viewer of evidence must not invent a date,
	// so the console shows "no date" instead.
	if sent, err := mail.ParseDate(msg.Header.Get("Date")); err == nil {
		e.Date = sent
	}
}

// headerLines reads the header block in message order, folded lines
// unfolded, a bare LF tolerated, a line without a colon skipped. The
// stdlib's map loses the order, which for a Received chain is the
// whole story.
func headerLines(raw []byte) []Header {
	head := raw
	if i := bytes.Index(raw, []byte("\r\n\r\n")); i >= 0 {
		head = raw[:i]
	} else if i := bytes.Index(raw, []byte("\n\n")); i >= 0 {
		head = raw[:i]
	}

	var out []Header
	for _, line := range strings.Split(strings.ReplaceAll(string(head), "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}

		if (line[0] == ' ' || line[0] == '\t') && len(out) > 0 {
			out[len(out)-1].Value += " " + strings.TrimSpace(line)

			continue
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}

		out = append(out, Header{Name: strings.TrimSpace(name), Value: strings.TrimSpace(value)})
	}

	for i := range out {
		out[i].Value = headerText(out[i].Value)
	}

	return out
}

// addBody files a decoded body under the type that described it.
//
// A message may carry more than one part of the same type - a
// multipart/mixed of two text/plain parts is legal and does happen - so
// a second one is appended rather than dropped or allowed to overwrite
// the first. Anything that is neither text nor html and was not claimed
// as an attachment is discarded: there is nowhere to put it.
func (e *Email) addBody(media, body string) {
	switch media {
	case contentTypeTextPlain:
		e.TextBody = joinBody(e.TextBody, body)
	case "text/html":
		e.HTMLBody = joinBody(e.HTMLBody, body)
	}
}

func joinBody(existing, next string) string {
	if existing == "" {
		return next
	}

	return existing + "\n" + next
}

// oneAddress reduces a From header to a bare address.
//
// The header as sent is kept when it cannot be parsed, because an
// unparseable From is still evidence of who sent this and throwing it
// away leaves the reader with nothing at all.
func oneAddress(v string) string {
	if v == "" {
		return ""
	}

	if addr, err := mail.ParseAddress(v); err == nil {
		return addr.Address
	}

	return v
}

// mailboxList renders To or Cc back as a header value, display names
// kept. Through mail.Address.String, so a name needing quotes or
// RFC 2047 encoding gets it and the result is safe to write again.
func mailboxList(v string) string {
	if v == "" {
		return ""
	}

	addrs, err := mail.ParseAddressList(v)
	if err != nil {
		return ""
	}

	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.String())
	}

	return strings.Join(out, ", ")
}

// addressList reduces To or Cc to bare addresses.
//
// Unlike From this answers nil on a parse failure rather than the raw
// header: the field is a LIST, and one string that happens to contain
// commas would be read downstream as a single recipient.
func addressList(v string) []string {
	if v == "" {
		return nil
	}

	addrs, err := mail.ParseAddressList(v)
	if err != nil {
		return nil
	}

	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Address)
	}

	return out
}
