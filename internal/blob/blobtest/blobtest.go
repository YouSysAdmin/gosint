// Gosint, Copyright (c) 2026 YouSysAdmin

// Package blobtest is the contract every blob backend has to meet.
//
// One suite, run by each backend's own test file against a store it
// opened: fs against a temp dir, s3 against the bucket named by the
// GOSINT_TEST_S3_* variables. The suite tests blob.Store, not a layout
// - where the bytes land is the backend's own test's business.
package blobtest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
)

// Opener returns a fresh, EMPTY store for one subtest.
type Opener func(t *testing.T) blob.Store

// Run runs the whole contract.
func Run(t *testing.T, open Opener) {
	t.Helper()

	ctx := t.Context()

	t.Run("put hashes the content and deduplicates", func(t *testing.T) {
		s := open(t)
		content := "\x89PNG\r\n\x1a\nfake"
		want := sha256.Sum256([]byte(content))

		hash, size, err := s.Put(ctx, strings.NewReader(content), 1024)
		if err != nil {
			t.Fatalf("Put: %v", err)
		}

		if hash != hex.EncodeToString(want[:]) || size != int64(len(content)) {
			t.Errorf("Put = %s %d, want the sha256 and %d", hash, size, len(content))
		}

		// The same bytes again: the same hash, and still ONE blob.
		again, _, err := s.Put(ctx, strings.NewReader(content), 1024)
		if err != nil || again != hash {
			t.Errorf("second Put = %s %v, want %s", again, err, hash)
		}

		other, _, err := s.Put(ctx, strings.NewReader("different"), 1024)
		if err != nil || other == hash {
			t.Errorf("Put of other content = %s %v", other, err)
		}

		var listed []string
		before := time.Now().Add(-time.Minute)
		if err := s.List(ctx, func(h string, modified time.Time) error {
			listed = append(listed, h)

			// The gc decides what is an orphan and what is an
			// upload in flight by this, so a backend that
			// answered the zero time would make it delete
			// bytes whose record has not landed yet.
			if modified.Before(before) {
				t.Errorf("List gave %s a modified time of %v", h, modified)
			}

			return nil
		}); err != nil {
			t.Fatalf("List: %v", err)
		}

		if len(listed) != 2 {
			t.Errorf("List = %v, want exactly two hashes", listed)
		}

		if ok, err := s.Exists(ctx, hash); !ok || err != nil {
			t.Errorf("Exists after Put = %v %v", ok, err)
		}

		if ok, err := s.Exists(ctx, strings.Repeat("0", blob.HashLen)); ok || err != nil {
			t.Errorf("Exists of an unknown hash = %v %v", ok, err)
		}

		if err := s.Ping(ctx); err != nil {
			t.Errorf("Ping: %v", err)
		}
	})

	t.Run("too large keeps nothing", func(t *testing.T) {
		s := open(t)
		_, _, err := s.Put(ctx, strings.NewReader("0123456789"), 9)
		if !errors.Is(err, blob.ErrTooLarge) {
			t.Fatalf("Put over the limit: %v, want ErrTooLarge", err)
		}

		// Exactly the limit is fine.
		if _, _, err := s.Put(ctx, strings.NewReader("012345678"), 9); err != nil {
			t.Errorf("Put at the limit: %v", err)
		}

		n := 0
		if err := s.List(ctx, func(string, time.Time) error { n++; return nil }); err != nil {
			t.Fatal(err)
		}

		if n != 1 {
			t.Errorf("%d blobs after a refused Put, want 1", n)
		}
	})

	t.Run("open whole and by range", func(t *testing.T) {
		s := open(t)
		content := "0123456789abcdef"
		hash, _, err := s.Put(ctx, strings.NewReader(content), 1024)
		if err != nil {
			t.Fatal(err)
		}

		read := func(rng *blob.Range) (string, blob.Object, error) {
			r, obj, err := s.Open(ctx, hash, rng)
			if err != nil {
				return "", obj, err
			}

			defer func() { _ = r.Close() }()

			b, err := io.ReadAll(r)

			return string(b), obj, err
		}

		got, obj, err := read(nil)
		if err != nil || got != content || obj.Size != 16 || obj.Offset != 0 || obj.Length != 16 {
			t.Errorf("Open whole = %q %+v %v", got, obj, err)
		}

		got, obj, err = read(&blob.Range{Start: 0, End: 3})
		if err != nil || got != "0123" || obj.Offset != 0 || obj.Length != 4 || obj.Size != 16 {
			t.Errorf("Open 0-3 = %q %+v %v", got, obj, err)
		}

		got, obj, err = read(&blob.Range{Start: 4, End: 7})
		if err != nil || got != "4567" || obj.Offset != 4 || obj.Length != 4 {
			t.Errorf("Open 4-7 = %q %+v %v", got, obj, err)
		}

		// Open-ended, and an end past the size, both clamp.
		got, obj, err = read(&blob.Range{Start: 12, End: -1})
		if err != nil || got != "cdef" || obj.Offset != 12 || obj.Length != 4 {
			t.Errorf("Open 12- = %q %+v %v", got, obj, err)
		}

		got, _, err = read(&blob.Range{Start: 14, End: 100})
		if err != nil || got != "ef" {
			t.Errorf("Open 14-100 = %q %v", got, err)
		}

		if _, _, err := read(&blob.Range{Start: 16, End: -1}); !errors.Is(err, blob.ErrRange) {
			t.Errorf("Open past the end: %v, want ErrRange", err)
		}

		if _, _, err := s.Open(ctx, strings.Repeat("a", blob.HashLen), nil); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Open of an unknown hash: %v, want ErrNotFound", err)
		}

		if _, _, err := s.Open(ctx, "../../etc/passwd", nil); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Open of a non-hash: %v, want ErrNotFound", err)
		}
	})

	t.Run("delete is idempotent", func(t *testing.T) {
		s := open(t)
		hash, _, err := s.Put(ctx, strings.NewReader("bytes"), 1024)
		if err != nil {
			t.Fatal(err)
		}

		if err := s.Delete(ctx, hash); err != nil {
			t.Fatalf("Delete: %v", err)
		}

		if ok, _ := s.Exists(ctx, hash); ok {
			t.Error("the blob survived Delete")
		}

		if _, _, err := s.Open(ctx, hash, nil); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Open after Delete: %v, want ErrNotFound", err)
		}

		if err := s.Delete(ctx, hash); err != nil {
			t.Errorf("second Delete: %v, want nil", err)
		}

		n := 0
		if err := s.List(ctx, func(string, time.Time) error { n++; return nil }); err != nil || n != 0 {
			t.Errorf("List after Delete = %d %v", n, err)
		}
	})
}
