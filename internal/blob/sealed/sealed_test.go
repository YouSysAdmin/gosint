// Gosint, Copyright (c) 2026 YouSysAdmin

package sealed_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/blob/blobtest"
	blobfs "github.com/yousysadmin/gosint/internal/blob/fs"
	"github.com/yousysadmin/gosint/internal/blob/sealed"
	"github.com/yousysadmin/gosint/internal/core/vault"
)

func open(t *testing.T) (*sealed.Store, *blobfs.Store, string) {
	t.Helper()

	dir := t.TempDir()
	inner, err := blobfs.New(dir)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	return sealed.New(inner, v), inner, dir
}

func TestContract(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		t.Helper()

		s, _, _ := open(t)

		return s
	})
}

// No byte of the content is on disk in clear, ranges across chunk
// edges read right, and the same content is stored once.
func TestBlobsAreSealedOnDisk(t *testing.T) {
	s, _, dir := open(t)
	ctx := t.Context()

	plain := bytes.Repeat([]byte("secret-marker-"), 3*sealed.Chunk/14+7)
	digest, size, err := s.Put(ctx, bytes.NewReader(plain), 1<<30)
	if err != nil || size != int64(len(plain)) {
		t.Fatal(size, err)
	}

	if again, _, _ := s.Put(ctx, bytes.NewReader(plain), 1<<30); again != digest {
		t.Error("the same content got another digest")
	}

	files := 0
	_ = filepath.WalkDir(filepath.Join(dir, blob.Dir), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		files++
		b, _ := os.ReadFile(path) //nolint:gosec // a test temp dir
		if bytes.Contains(b, []byte("secret-marker")) || strings.Contains(path, digest) {
			t.Errorf("%s holds the content or its hash", path)
		}

		return nil
	})

	if files != 1 {
		t.Errorf("%d files, want one", files)
	}

	for _, rng := range []blob.Range{{Start: sealed.Chunk - 3, End: sealed.Chunk + 5}, {Start: 2*sealed.Chunk + 1, End: -1}, {Start: 0, End: 0}} {
		r, obj, err := s.Open(ctx, digest, &rng)
		if err != nil {
			t.Fatalf("range %+v: %v", rng, err)
		}

		got, err := io.ReadAll(r)
		_ = r.Close()

		end := rng.End
		if end < 0 {
			end = int64(len(plain)) - 1
		}

		if err != nil || !bytes.Equal(got, plain[rng.Start:end+1]) || obj.Offset != rng.Start || obj.Size != int64(len(plain)) {
			t.Errorf("range %+v = %d bytes %+v %v", rng, len(got), obj, err)
		}
	}
}

// A switch holds the contract once set, refuses before, and a new
// backend takes the writes after it while the old one keeps its own.
func TestASwitchChangesBackend(t *testing.T) {
	v, err := vault.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}

	sw := sealed.NewSwitch(v)
	if _, _, err := sw.Put(t.Context(), strings.NewReader("x"), 10); !errors.Is(err, sealed.ErrNoBackend) {
		t.Fatalf("a put with no backend = %v", err)
	}

	blobtest.Run(t, func(t *testing.T) blob.Store {
		t.Helper()

		inner, err := blobfs.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		s := sealed.NewSwitch(v)
		s.Set(inner, "fs")

		return s
	})

	first, err := blobfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	sw.Set(first, "first")
	hash, _, err := sw.Put(t.Context(), strings.NewReader("kept"), 10)
	if err != nil {
		t.Fatal(err)
	}

	second, err := blobfs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	sw.Set(second, "second")
	if sw.Key() != "second" {
		t.Fatalf("key = %s", sw.Key())
	}

	if ok, err := sw.Exists(t.Context(), hash); err != nil || ok {
		t.Fatalf("the new backend holds the old file: %v %v", ok, err)
	}
}
