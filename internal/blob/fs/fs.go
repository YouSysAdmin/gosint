// Gosint, Copyright (c) 2026 YouSysAdmin

// Package fs keeps blobs as files beside the database. The default,
// and the one that makes a file something an operator can find, back
// up and restore with the tools they already have.
package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
)

// Store is the on-disk implementation.
type Store struct {
	root string
}

// New roots the store at dir, creating it and its spool directory.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o750); err != nil {
		return nil, fmt.Errorf("create blobs dir: %w", err)
	}

	return &Store{root: dir}, nil
}

// path is where one blob lives. The hash is checked before it becomes
// a path, because it arrives from the store and a store can be
// edited.
func (s *Store) path(hash string) (string, error) {
	if !blob.ValidHash(hash) {
		return "", blob.ErrBadHash
	}

	return filepath.Join(s.root, filepath.FromSlash(blob.Shard(hash))), nil
}

// Put spools the upload beside the tree, then links it into place
// under its hash. A blob that is already there wins: the spool is
// discarded and the caller is told the same hash, which is the
// deduplication.
func (s *Store) Put(ctx context.Context, r io.Reader, limit int64) (string, int64, error) {
	return s.put(ctx, "", r, limit)
}

// PutNamed is Put under a name the caller chose.
func (s *Store) PutNamed(ctx context.Context, name string, r io.Reader, limit int64) (int64, error) {
	if !blob.ValidHash(name) {
		return 0, blob.ErrBadHash
	}

	_, size, err := s.put(ctx, name, r, limit)

	return size, err
}

// put stores r under name, its own hash when name is empty.
func (s *Store) put(ctx context.Context, name string, r io.Reader, limit int64) (string, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	f, hash, size, err := blob.Spool(filepath.Join(s.root, "tmp"), r, limit)
	if err != nil {
		return "", 0, err
	}

	if name != "" {
		hash = name
	}

	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	dst, err := s.path(hash)
	if err != nil {
		return "", 0, err
	}

	if _, err := os.Lstat(dst); err == nil {
		return hash, size, touch(dst)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return "", 0, err
	}

	if err := os.Chmod(f.Name(), 0o640); err != nil { //nolint:gosec // group-readable, for a backup user
		return "", 0, err
	}

	// On disk before the name points at it. The record that follows
	// is fsynced by the store on commit, so without this a power loss
	// can leave a durable record naming an empty or partial file - one
	// served under a hash it does not match, and never repaired,
	// because a re-upload finds the name taken.
	if err := f.Sync(); err != nil {
		return "", 0, fmt.Errorf("sync the spooled blob: %w", err)
	}

	// Link, not rename: two uploads of the same content racing here
	// both succeed, one by linking and one by finding the file there.
	// Some filesystems refuse hard links, and a rename then loses nothing
	// but the exclusivity, and the content under the name is the same
	// bytes either way.
	if err := os.Link(f.Name(), dst); err != nil {
		if errors.Is(err, os.ErrExist) {
			return hash, size, touch(dst)
		}

		if err := os.Rename(f.Name(), dst); err != nil {
			return "", 0, err
		}
	}

	if err := syncDir(filepath.Dir(dst)); err != nil {
		return "", 0, err
	}

	return hash, size, nil
}

// touch moves a blob's modification time to now when an upload finds
// it already there. The gc judges an orphan by that time, and a blob
// nothing named an hour ago is exactly what this upload is about to
// name - without the touch it looks old and unwanted until the record
// lands.
func touch(path string) error {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("touch the blob: %w", err)
	}

	return nil
}

// syncDir makes a new directory entry durable. A file fsynced under a
// name that was never fsynced can vanish whole in a power loss.
func syncDir(dir string) error {
	d, err := os.Open(dir) //nolint:gosec // a directory this store made
	if err != nil {
		return fmt.Errorf("open the shard directory: %w", err)
	}

	defer func() { _ = d.Close() }()

	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync the shard directory: %w", err)
	}

	return nil
}

// Open returns the file, or the part of it rng names.
func (s *Store) Open(ctx context.Context, hash string, rng *blob.Range) (io.ReadCloser, blob.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, blob.Object{}, err
	}

	path, err := s.path(hash)
	if err != nil {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	// Only a missing file is a missing blob. A descriptor limit or a
	// permission problem is the operator's to fix, and reading it as
	// "the record has no bytes" points them at the wrong thing.
	f, err := os.Open(path) //nolint:gosec // path is the validated hash under the root
	if errors.Is(err, fs.ErrNotExist) {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	if err != nil {
		return nil, blob.Object{}, fmt.Errorf("open the blob: %w", err)
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return nil, blob.Object{}, fmt.Errorf("stat the blob: %w", err)
	}

	if st.IsDir() {
		_ = f.Close()

		return nil, blob.Object{}, blob.ErrNotFound
	}

	obj := blob.Object{Size: st.Size(), Offset: 0, Length: st.Size()}
	if rng == nil {
		return f, obj, nil
	}

	offset, length, ok := rng.Resolve(st.Size())
	if !ok {
		_ = f.Close()

		return nil, obj, blob.ErrRange
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		_ = f.Close()

		return nil, obj, err
	}

	obj.Offset = offset
	obj.Length = length

	return &section{Reader: io.LimitReader(f, length), f: f}, obj, nil
}

// section is a bounded view of a file that closes the file.
type section struct {
	io.Reader
	f *os.File
}

func (s *section) Close() error { return s.f.Close() }

// Exists reports whether the file is there.
func (s *Store) Exists(ctx context.Context, hash string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// A malformed hash names no file, so it reads as missing rather
	// than as a failure of the store.
	path, err := s.path(hash)
	if err != nil {
		return false, nil
	}

	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}

		return false, err
	}

	return !st.IsDir(), nil
}

// Delete removes the file. Missing is fine.
func (s *Store) Delete(ctx context.Context, hash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// A malformed hash names no file, and deleting what is not there
	// is not an error.
	path, err := s.path(hash)
	if err != nil {
		return nil
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return nil
}

// List walks the assets tree. A file whose name is not a hash is
// skipped, not reported: it is somebody's stray, not a blob.
func (s *Store) List(ctx context.Context, fn func(hash string, modified time.Time) error) error {
	root := filepath.Join(s.root, blob.Dir)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}

			return err
		}

		if err := ctx.Err(); err != nil {
			return err
		}

		if d.IsDir() || !blob.ValidHash(d.Name()) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			// Removed between the walk and the stat, which is a
			// blob that is no longer there to report.
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}

			return err
		}

		return fn(d.Name(), info.ModTime().UTC())
	})
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

// Ping checks that the root is a directory this process can list.
func (s *Store) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	st, err := os.Stat(s.root)
	if err != nil {
		return err
	}

	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", s.root)
	}

	return nil
}
