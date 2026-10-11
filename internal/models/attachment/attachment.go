// Gosint, Copyright (c) 2026 YouSysAdmin

// Package attachment is a file attached to an entity: the record. The
// bytes live in the blob store under Hash.
package attachment

import "time"

// Attachment is one file on one node.
type Attachment struct {
	ID       string `json:"id"`
	SketchID string `json:"sketch_id"`
	NodeID   string `json:"node_id"`

	// Name is the file name as uploaded, blob.ValidName says what is
	// allowed. Two files of one node may share a name.
	Name string `json:"name"`

	// ContentType was sniffed at upload, from the name first, and is
	// what the file is served as.
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`

	// Hash is the sha256 of the content, the key of the bytes. Stored
	// sealed, the record matched by a keyed hash of it.
	Hash       string    `json:"hash"`
	UploaderID string    `json:"uploader_id"`
	CreatedAt  time.Time `json:"created_at"`

	// Metadata is what the file says about itself (filemeta), read at
	// upload. Nil when not read yet, empty when it says nothing.
	Metadata map[string]any `json:"metadata"`
}

// IsImage says whether the file draws as a picture.
func (a Attachment) IsImage() bool {
	switch a.ContentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/avif", "image/bmp":
		return true
	}

	return false
}
