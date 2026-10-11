// Gosint, Copyright (c) 2026 YouSysAdmin

// Package blob is where the BYTES of files attached to entities live,
// addressed by the sha256 of their content. Ported from iwiki.
//
// Content addressing is the whole design. A blob has no name and no
// owner: the record that says "screenshot.png on entity X is blob
// ab12..." lives in the store (store.AttachmentStore), and two records
// with the same content share one blob. That gives deduplication for
// free, makes every write idempotent (writing a blob that exists is a
// no-op, whoever wins the race), and turns the on-disk layout into
// assets/ab/cd/<hash> - at most 65536 leaf directories however many
// files an install accumulates.
//
// Three backends: the local filesystem under the data directory, an
// S3-compatible bucket, and the database itself for a postgres fleet
// without a bucket. All are served THROUGH the server, so the access
// gates apply to a file as they do to its sketch. A blob store does
// not decide when a blob is no longer wanted: the store counts
// references and the domain deletes at zero.
package blob

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Store is the operations the asset domain and the gc need.
type Store interface {
	// Put reads r to the end, at most limit bytes, and stores the
	// content under its sha256. Returns the hash and the size. A blob
	// that already exists is left alone and reported as if written.
	// ErrTooLarge when r held more than limit, and nothing is kept.
	Put(ctx context.Context, r io.Reader, limit int64) (hash string, size int64, err error)

	// Open returns the content, or the part of it rng names. The
	// caller closes it. ErrNotFound when it is missing, ErrRange when
	// rng starts past the end.
	Open(ctx context.Context, hash string, rng *Range) (io.ReadCloser, Object, error)

	// Exists reports whether the blob is there.
	Exists(ctx context.Context, hash string) (bool, error)

	// Delete removes the blob. A missing one is not an error: the
	// caller is reconciling, not asserting.
	Delete(ctx context.Context, hash string) error

	// List calls fn with every stored hash and when it was written,
	// in no particular order, stopping at the first error fn
	// returns.
	//
	// The TIME is what makes the gc safe to run on a live fleet.
	// Bytes are written before the record that names them, so an
	// upload in flight looks exactly like an orphan - and the only
	// thing telling them apart is that one of them just arrived.
	List(ctx context.Context, fn func(hash string, modified time.Time) error) error

	// Ping reports whether the backing store answers. For /readyz.
	Ping(ctx context.Context) error
}

// NamedPutter stores bytes under a name the caller chose rather than
// their hash, which a layer sealing the bytes needs: its name is a keyed
// hash of the content, not the hash of what it stores. Every backend
// implements it.
type NamedPutter interface {
	// PutNamed is Put under name, a lower-case hex string of HashLen. A
	// blob already there under the name is left alone.
	PutNamed(ctx context.Context, name string, r io.Reader, limit int64) (int64, error)
}

// Object describes what Open returned.
type Object struct {
	// Size is the whole blob's size, whatever range was asked for.
	Size int64

	// Offset and Length are the part being returned. For a full read
	// Offset is 0 and Length is Size.
	Offset int64
	Length int64
}

// Range is an inclusive byte range, the way HTTP spells it. End
// below zero means "to the end".
type Range struct {
	Start int64
	End   int64
}

// Resolve clamps the range against a size and returns the offset and
// length to read. false when the start is past the end, which HTTP
// answers with 416.
func (r Range) Resolve(size int64) (offset, length int64, ok bool) {
	if r.Start < 0 || r.Start >= size {
		return 0, 0, false
	}

	end := r.End
	if end < 0 || end >= size {
		end = size - 1
	}

	if end < r.Start {
		return 0, 0, false
	}

	return r.Start, end - r.Start + 1, true
}

// The sentinel errors.
var (
	// ErrNotFound says no blob is stored under the hash. Only a
	// missing blob is this - a store that cannot be read returns its
	// own error, so a permission problem is not mistaken for a hole.
	ErrNotFound = errors.New("blob not found")

	// ErrTooLarge says Put read past its limit, and nothing was kept.
	ErrTooLarge = errors.New("the attachment is larger than allowed")

	// ErrRange says a range starts past the end of the blob, which
	// HTTP answers with 416.
	ErrRange = errors.New("the requested range is not satisfiable")

	// ErrBadHash says a string is not a lower-case hex sha256 and so
	// cannot become a path or a key.
	ErrBadHash = errors.New("the blob hash is not a sha256")

	// ErrBadName says a name failed ValidName.
	ErrBadName = errors.New("the attachment name is not allowed")
)

// HashLen is the length of a lower-case hex sha256.
const HashLen = 64

// ValidHash reports whether h is a lower-case hex sha256. Checked
// before a hash becomes a path or a key, because one arrives from the
// store and a store can be edited.
func ValidHash(h string) bool {
	if len(h) != HashLen {
		return false
	}

	for i := range len(h) {
		c := h[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}

	return true
}

// MaxNameBytes bounds a file name.
const MaxNameBytes = 200

// ValidName reports why name may not be an attachment name, or nil.
//
// The name is a path segment in a URL and a key part in the store, so
// nothing that could be read as a path or hide the file is allowed:
// no separators, no leading dot, no control characters, no ".." and
// nothing reserved by a filesystem somebody will export the files to.
func ValidName(name string) error {
	if name == "" || len(name) > MaxNameBytes || !utf8.ValidString(name) {
		return ErrBadName
	}

	if name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return ErrBadName
	}

	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return ErrBadName
	}

	for _, r := range name {
		if unicode.IsControl(r) {
			return ErrBadName
		}
	}

	// Windows drops a trailing dot or space, and keeps device names
	// whatever the extension.
	if strings.TrimSpace(name) != name || strings.HasSuffix(name, ".") {
		return ErrBadName
	}

	base, _, _ := strings.Cut(name, ".")
	if reservedName(strings.TrimSpace(base)) {
		return ErrBadName
	}

	return nil
}

// reservedName is a Windows device name: CON, PRN, AUX, NUL, COM1 to
// COM9, LPT1 to LPT9, in any case.
func reservedName(base string) bool {
	b := strings.ToUpper(base)
	switch b {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}

	if len(b) == 4 && (strings.HasPrefix(b, "COM") || strings.HasPrefix(b, "LPT")) {
		return b[3] >= '1' && b[3] <= '9'
	}

	return false
}
