// Gosint, Copyright (c) 2026 YouSysAdmin

package attachment

import (
	"time"

	amodel "github.com/yousysadmin/gosint/internal/models/attachment"
)

// File is one attached file on the wire, with where to fetch it.
type File struct {
	amodel.Attachment

	// URL serves the bytes, same origin, so the console draws it
	// without any cross-origin trouble.
	URL string `json:"url"`

	// IsImage says the console may show it as a picture.
	IsImage bool `json:"is_image"`
}

// fileResponse answers one file.
type fileResponse struct {
	File File `json:"file"`
}

// listResponse answers a node's files.
type listResponse struct {
	Files []File `json:"files"`
}

// MailHeader is one header of a message, in message order.
type MailHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// MailPart is one attachment of a message, fetched by its URL.
type MailPart struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`

	// ContentID is what the HTML body names in a cid: URL.
	ContentID string `json:"content_id,omitzero"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
}

// Mail is an attached .eml as the viewer shows it. Date is zero when
// the message carries none.
type Mail struct {
	From        string       `json:"from"`
	FromName    string       `json:"from_name"`
	To          []string     `json:"to"`
	Cc          []string     `json:"cc"`
	Date        time.Time    `json:"date,omitzero"`
	Subject     string       `json:"subject"`
	MessageID   string       `json:"message_id"`
	Received    []string     `json:"received"`
	Headers     []MailHeader `json:"headers"`
	Text        string       `json:"text"`
	HTML        string       `json:"html"`
	Attachments []MailPart   `json:"attachments"`
}

// mailResponse answers a parsed message.
type mailResponse struct {
	Mail Mail `json:"mail"`
}
