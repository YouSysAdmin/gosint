// Gosint, Copyright (c) 2026 YouSysAdmin

package fs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
	"github.com/yousysadmin/gosint/internal/blob/blobtest"
)

// The contract, against a temp dir per subtest.
func TestContract(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		t.Helper()

		s, err := New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		return s
	})
}

// The layout is this backend's own promise: assets/ab/cd/<hash>, and
// no spool file left behind, so an operator who looks finds what the
// documentation says.
func TestLayoutIsShardedAndTheSpoolIsClean(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	hash, _, err := s.Put(t.Context(), strings.NewReader("hello"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "assets", hash[0:2], hash[2:4], hash)
	if _, err := os.Stat(want); err != nil {
		t.Errorf("blob not at %s: %v", want, err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "tmp"))
	if err != nil || len(entries) != 0 {
		t.Errorf("spool dir has %d entries after Put, want 0 (err %v)", len(entries), err)
	}

	// A refused upload leaves nothing in the spool either.
	if _, _, err := s.Put(t.Context(), strings.NewReader("too long"), 3); err == nil {
		t.Fatal("Put over the limit succeeded")
	}

	entries, _ = os.ReadDir(filepath.Join(dir, "tmp"))
	if len(entries) != 0 {
		t.Errorf("spool dir has %d entries after a refused Put", len(entries))
	}
}

// An upload that finds its bytes already stored makes them look new.
//
// What it caught: the gc removes a blob nothing names once it is an
// hour old, and an upload deduplicated onto such a blob left it old -
// so the gc could take it in the moment before the record landed.
func TestDeduplicatedPutRefreshesTheTime(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	hash, _, err := s.Put(t.Context(), strings.NewReader("hello"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "assets", hash[0:2], hash[2:4], hash)
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	if _, _, err := s.Put(t.Context(), strings.NewReader("hello"), 1024); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	if st.ModTime().Before(time.Now().Add(-time.Minute)) {
		t.Errorf("the deduplicated blob kept its old time %v", st.ModTime())
	}
}

// A blob the process cannot read is a failure, not a missing file.
// Reading it as ErrNotFound logs "a record with no blob" and sends an
// operator looking for a backup when the problem is a permission.
func TestOpenReportsAnUnreadableBlobAsAnError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file whatever its mode")
	}

	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	hash, _, err := s.Put(t.Context(), strings.NewReader("hello"), 1024)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "assets", hash[0:2], hash[2:4], hash)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(path, 0o640) }) //nolint:gosec // test

	_, _, err = s.Open(t.Context(), hash, nil)
	if err == nil || errors.Is(err, blob.ErrNotFound) {
		t.Errorf("Open of an unreadable blob = %v, want an error that is not ErrNotFound", err)
	}
}
