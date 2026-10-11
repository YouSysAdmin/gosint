// Gosint, Copyright (c) 2026 YouSysAdmin

package db

import (
	"bytes"
	"io"
)

// bytesReader is a reader over an owned slice.
func bytesReader(b []byte) io.Reader {
	return bytes.NewReader(b)
}
