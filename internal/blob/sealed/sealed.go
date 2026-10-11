// Gosint, Copyright (c) 2026 YouSysAdmin

// Package sealed encrypts blobs before they reach a backend.
//
// The callers keep addressing a blob by the sha256 of its content (the
// digest), which records, dedup and the release marks name. The backend
// holds it under a keyed hash of the digest, so a known file cannot be
// confirmed by its hash, and holds ciphertext: AES-256-GCM in chunks of
// 64 KiB under a key derived from the master key and the digest. The
// key and the nonces are fixed by the content, so the same file seals
// to the same bytes and is still stored once, and only the master key
// makes them. A chunk's nonce carries its index and a last chunk flag,
// so chunks cannot be reordered, dropped or cut off.
package sealed

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/yousysadmin/gosint/internal/blob"
)

// Chunk is how many plain bytes one sealed chunk holds.
const Chunk = 64 * 1024

const (
	magic    = "GSB1"
	tagSize  = 16
	sealedCK = Chunk + tagSize
)

// NamePurpose is the keyed hash a blob is stored under. A file record
// holds the same one, so it names the stored blob without the sha256.
// Changing it orphans every stored blob.
const NamePurpose = "gosint/blob-name"

const keyPurpose = "gosint/blob-key"

// Keys derives the keys blobs are named and sealed with. The vault
// implements it.
type Keys interface {
	Derive(info string) ([]byte, error)
}

// Inner is a backend that stores under a chosen name.
type Inner interface {
	blob.Store
	blob.NamedPutter
}

// Store seals what it stores in Inner.
type Store struct {
	inner Inner
	keys  Keys
}

// New wraps inner.
func New(inner Inner, keys Keys) *Store {
	return &Store{inner: inner, keys: keys}
}

var _ blob.Store = (*Store)(nil)

func (s *Store) mac(purpose, digest string) ([]byte, error) {
	k, err := s.keys.Derive(purpose)
	if err != nil {
		return nil, err
	}

	m := hmac.New(sha256.New, k)
	m.Write([]byte(digest))

	return m.Sum(nil), nil
}

// Name is where the blob of digest is stored.
func (s *Store) Name(digest string) (string, error) {
	if !blob.ValidHash(digest) {
		return "", blob.ErrBadHash
	}

	sum, err := s.mac(NamePurpose, digest)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(sum), nil
}

func (s *Store) aead(digest string) (cipher.AEAD, error) {
	key, err := s.mac(keyPurpose, digest)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// nonce is chunk index's nonce. The index is never negative.
func nonce(index int64, last bool) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n, uint64(index)) //nolint:gosec // a chunk index, never negative
	if last {
		n[11] = 1
	}

	return n
}

// chunks is how many chunks n plain bytes take, one at least.
func chunks(n int64) int64 {
	return max(1, (n+Chunk-1)/Chunk)
}

// plainSize is the plain size of a sealed blob of size bytes.
func plainSize(size int64) (int64, error) {
	body := size - int64(len(magic))
	if body < tagSize {
		return 0, errors.New("sealed blob: too short")
	}

	count := (body + sealedCK - 1) / sealedCK

	return body - tagSize*count, nil
}

// Seal is plain sealed as stored, for digest.
func (s *Store) Seal(digest string, plain []byte) ([]byte, error) {
	gcm, err := s.aead(digest)
	if err != nil {
		return nil, err
	}

	count := chunks(int64(len(plain)))
	out := make([]byte, 0, len(magic)+len(plain)+int(count)*tagSize)
	out = append(out, magic...)
	for i := range count {
		from := i * Chunk
		to := min(from+Chunk, int64(len(plain)))
		out = gcm.Seal(out, nonce(i, i == count-1), plain[from:to], nil)
	}

	return out, nil
}

// Put reads r, at most limit bytes, and stores it sealed. It answers
// the digest, the content's sha256, and the plain size.
func (s *Store) Put(ctx context.Context, r io.Reader, limit int64) (string, int64, error) {
	plain, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return "", 0, err
	}

	if int64(len(plain)) > limit {
		return "", 0, blob.ErrTooLarge
	}

	sum := sha256.Sum256(plain)
	digest := hex.EncodeToString(sum[:])
	if err := s.put(ctx, digest, plain); err != nil {
		return "", 0, err
	}

	return digest, int64(len(plain)), nil
}

func (s *Store) put(ctx context.Context, digest string, plain []byte) error {
	name, err := s.Name(digest)
	if err != nil {
		return err
	}

	sealed, err := s.Seal(digest, plain)
	if err != nil {
		return err
	}

	_, err = s.inner.PutNamed(ctx, name, bytes.NewReader(sealed), int64(len(sealed)))

	return err
}

// Open answers the plain content of digest, or the part rng names.
func (s *Store) Open(ctx context.Context, digest string, rng *blob.Range) (io.ReadCloser, blob.Object, error) {
	name, err := s.Name(digest)
	if err != nil {
		return nil, blob.Object{}, blob.ErrNotFound
	}

	var first int64
	if rng != nil && rng.Start > 0 {
		first = rng.Start / Chunk
	}

	var at *blob.Range
	if first > 0 {
		at = &blob.Range{Start: int64(len(magic)) + first*sealedCK, End: -1}
	}

	r, obj, err := s.inner.Open(ctx, name, at)
	if err != nil {
		return nil, blob.Object{}, err
	}

	size, err := plainSize(obj.Size)
	if err != nil {
		_ = r.Close()

		return nil, blob.Object{}, err
	}

	out := blob.Object{Size: size, Offset: 0, Length: size}
	if rng != nil {
		offset, length, ok := rng.Resolve(size)
		if !ok {
			_ = r.Close()

			return nil, blob.Object{Size: size}, blob.ErrRange
		}

		out.Offset, out.Length = offset, length
	}

	gcm, err := s.aead(digest)
	if err != nil {
		_ = r.Close()

		return nil, blob.Object{}, err
	}

	if first == 0 {
		head := make([]byte, len(magic))
		if _, err := io.ReadFull(r, head); err != nil || string(head) != magic {
			_ = r.Close()

			return nil, blob.Object{}, fmt.Errorf("sealed blob %s: bad header", name)
		}
	}

	return &reader{src: r, gcm: gcm, index: first, count: chunks(size),
		skip: out.Offset - first*Chunk, left: out.Length}, out, nil
}

// Exists reports whether the blob is stored.
func (s *Store) Exists(ctx context.Context, digest string) (bool, error) {
	name, err := s.Name(digest)
	if err != nil {
		return false, nil
	}

	return s.inner.Exists(ctx, name)
}

// Delete removes the blob.
func (s *Store) Delete(ctx context.Context, digest string) error {
	name, err := s.Name(digest)
	if err != nil {
		return nil
	}

	return s.inner.Delete(ctx, name)
}

// DeleteStored removes a blob by the name it is stored under.
func (s *Store) DeleteStored(ctx context.Context, name string) error {
	return s.inner.Delete(ctx, name)
}

// List lists what the backend holds: the stored names, not digests.
func (s *Store) List(ctx context.Context, fn func(hash string, modified time.Time) error) error {
	return s.inner.List(ctx, fn)
}

// Ping asks the backend.
func (s *Store) Ping(ctx context.Context) error {
	return s.inner.Ping(ctx)
}

// reader opens chunks as they are read.
type reader struct {
	src   io.ReadCloser
	gcm   cipher.AEAD
	index int64
	count int64
	skip  int64
	left  int64
	buf   []byte
}

func (r *reader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}

	for len(r.buf) == 0 {
		if r.index >= r.count {
			return 0, io.EOF
		}

		if err := r.next(); err != nil {
			return 0, err
		}
	}

	n := copy(p, r.buf[:min(int64(len(r.buf)), r.left)])
	r.buf = r.buf[n:]
	r.left -= int64(n)

	return n, nil
}

func (r *reader) next() error {
	sealed := make([]byte, sealedCK)
	n, err := io.ReadFull(r.src, sealed)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}

	last := r.index == r.count-1
	plain, err := r.gcm.Open(sealed[:0], nonce(r.index, last), sealed[:n], nil)
	if err != nil {
		return fmt.Errorf("sealed blob: chunk %d: %w", r.index, err)
	}

	r.index++
	if r.skip > 0 {
		plain = plain[min(r.skip, int64(len(plain))):]
		r.skip = 0
	}

	r.buf = plain

	return nil
}

func (r *reader) Close() error { return r.src.Close() }
