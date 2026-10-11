// Gosint, Copyright (c) 2026 YouSysAdmin

package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// Spool reads r into a temporary file under dir while hashing it,
// refusing more than limit bytes. Both backends need it: the key of a
// blob is its hash, and the hash is not known until the last byte has
// gone by, so the bytes have to land somewhere first.
//
// The file is returned open and positioned at the start. The caller
// owns it and removes it. On an error nothing is left behind.
func Spool(dir string, r io.Reader, limit int64) (*os.File, string, int64, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", 0, fmt.Errorf("create spool dir: %w", err)
	}

	f, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return nil, "", 0, err
	}

	// fail is the one exit for everything after this point: the spool
	// file is removed and the caller gets nothing to clean up.
	fail := func(err error) (*os.File, string, int64, error) {
		_ = f.Close()
		_ = os.Remove(f.Name())

		return nil, "", 0, err
	}

	// One byte past the limit says "too large" without reading the
	// rest of a body that is going to be refused anyway.
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r, limit+1))
	if err != nil {
		return fail(err)
	}

	if size > limit {
		return fail(ErrTooLarge)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}

	return f, hex.EncodeToString(h.Sum(nil)), size, nil
}

// Dir is the directory every blob lives under, in the fs root and in
// the bucket alike. Both backends list from here, so a stray object
// beside it is never mistaken for a blob.
const Dir = "assets"

// Shard is the relative key of a blob: assets/ab/cd/<hash>. Two
// levels of two hex characters each, so a store of a million blobs
// has 65536 leaf directories of about fifteen entries.
func Shard(hash string) string {
	return Dir + "/" + hash[0:2] + "/" + hash[2:4] + "/" + hash
}
