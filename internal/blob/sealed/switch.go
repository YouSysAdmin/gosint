// Gosint, Copyright (c) 2026 YouSysAdmin

package sealed

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
)

// ErrNoBackend is a file read or written before the server knows where
// files live, which it reads from its settings at the unseal.
var ErrNoBackend = errors.New("files: the file store is not ready, unseal the server")

// Switch is the sealed store over a backend an administrator replaces
// while the server runs. Key names the backend's location, so a refresh
// that finds it unchanged keeps the open one.
type Switch struct {
	keys Keys
	cur  atomic.Pointer[switched]
}

type switched struct {
	store *Store
	key   string
}

// NewSwitch starts with no backend.
func NewSwitch(keys Keys) *Switch { return &Switch{keys: keys} }

var _ blob.Store = (*Switch)(nil)

// Key is the location of the backend in use, empty for none.
func (s *Switch) Key() string {
	if c := s.cur.Load(); c != nil {
		return c.key
	}

	return ""
}

// Set seals into inner from now on, key naming its location.
func (s *Switch) Set(inner Inner, key string) {
	s.cur.Store(&switched{store: New(inner, s.keys), key: key})
}

func (s *Switch) store() (*Store, error) {
	c := s.cur.Load()
	if c == nil {
		return nil, ErrNoBackend
	}

	return c.store, nil
}

// Put implements blob.Store.
func (s *Switch) Put(ctx context.Context, r io.Reader, limit int64) (string, int64, error) {
	st, err := s.store()
	if err != nil {
		return "", 0, err
	}

	return st.Put(ctx, r, limit)
}

// Open implements blob.Store.
func (s *Switch) Open(ctx context.Context, digest string, rng *blob.Range) (io.ReadCloser, blob.Object, error) {
	st, err := s.store()
	if err != nil {
		return nil, blob.Object{}, err
	}

	return st.Open(ctx, digest, rng)
}

// Exists implements blob.Store.
func (s *Switch) Exists(ctx context.Context, digest string) (bool, error) {
	st, err := s.store()
	if err != nil {
		return false, err
	}

	return st.Exists(ctx, digest)
}

// Delete implements blob.Store.
func (s *Switch) Delete(ctx context.Context, digest string) error {
	st, err := s.store()
	if err != nil {
		return err
	}

	return st.Delete(ctx, digest)
}

// DeleteStored deletes by the stored name.
func (s *Switch) DeleteStored(ctx context.Context, name string) error {
	st, err := s.store()
	if err != nil {
		return err
	}

	return st.DeleteStored(ctx, name)
}

// List implements blob.Store.
func (s *Switch) List(ctx context.Context, fn func(hash string, modified time.Time) error) error {
	st, err := s.store()
	if err != nil {
		return err
	}

	return st.List(ctx, fn)
}

// Ping implements blob.Store.
func (s *Switch) Ping(ctx context.Context) error {
	st, err := s.store()
	if err != nil {
		return err
	}

	return st.Ping(ctx)
}
