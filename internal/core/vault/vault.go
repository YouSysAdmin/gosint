// Gosint, Copyright (c) 2026 YouSysAdmin

// Package vault encrypts what gosint stores, under one master key held
// in memory only.
//
// Each value gets a fresh 16 byte salt and 12 byte IV, a data key of
// HKDF-SHA256(master, salt, info = owner id), AES-256-GCM with the
// owner id as additional data. A value therefore only opens for the
// owner, row or purpose it was sealed for.
//
// A vault starts sealed: nothing opens until Unseal hands it the master
// key, which the server unwraps from the database with an unseal key.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	keySize  = 32
	saltSize = 16
	ivSize   = 12
	tagSize  = 16
)

// KeyVersion labels what every secret is sealed under. It is stored
// with each secret, so a later key rotation can tell old ones apart.
const KeyVersion = "V1"

// ErrUnknownVersion is a secret sealed under a master key this process
// does not know.
var ErrUnknownVersion = errors.New("vault: secret sealed under an unknown master key version")

// Sealed is one encrypted secret as stored.
type Sealed struct {
	Ciphertext []byte
	IV         []byte
	Salt       []byte
	Version    string
}

// ErrSealed is any use of a vault that holds no master key yet.
var ErrSealed = errors.New("vault: sealed")

// Vault seals and opens values under one master key.
type Vault struct {
	master atomic.Pointer[[]byte]

	once  sync.Once
	ready chan struct{}
}

// NewSealed builds a vault holding no key, waiting for Unseal.
func NewSealed() *Vault {
	return &Vault{ready: make(chan struct{})}
}

// New builds a vault already unsealed with a 32 byte master key.
func New(master []byte) (*Vault, error) {
	v := NewSealed()
	if err := v.Unseal(master); err != nil {
		return nil, err
	}

	return v, nil
}

// NewMaster is a fresh random master key.
func NewMaster() []byte {
	key := make([]byte, keySize)
	_, _ = rand.Read(key)

	return key
}

// Unseal hands the vault its master key.
func (v *Vault) Unseal(master []byte) error {
	if len(master) != keySize {
		return fmt.Errorf("vault: master key is %d bytes, want %d", len(master), keySize)
	}

	key := append([]byte(nil), master...)
	v.master.Store(&key)
	v.once.Do(func() { close(v.ready) })

	return nil
}

// Reseal forgets the master key. What was opened stays opened, nothing
// new opens until the next Unseal. The key is dropped, not zeroed: a
// call in flight still holds it, and zeroing it under that call would
// seal its value under a broken key.
func (v *Vault) Reseal() {
	v.master.Store(nil)
}

// IsSealed reports whether the vault holds no key.
func (v *Vault) IsSealed() bool {
	return v.master.Load() == nil
}

// Ready is closed by the first Unseal.
func (v *Vault) Ready() <-chan struct{} {
	return v.ready
}

func (v *Vault) key() ([]byte, error) {
	k := v.master.Load()
	if k == nil {
		return nil, ErrSealed
	}

	return *k, nil
}

// Seal encrypts plaintext for ownerID.
func (v *Vault) Seal(ownerID, plaintext string) (Sealed, error) {
	s := Sealed{Salt: make([]byte, saltSize), IV: make([]byte, ivSize), Version: KeyVersion}
	_, _ = rand.Read(s.Salt)
	_, _ = rand.Read(s.IV)

	gcm, err := v.aead(s.Salt, ownerID)
	if err != nil {
		return Sealed{}, err
	}

	s.Ciphertext = gcm.Seal(nil, s.IV, []byte(plaintext), []byte(ownerID))

	return s, nil
}

// Encode packs a Sealed into one string, for a TEXT column or a
// cookie: version first, then salt, iv and ciphertext in base64url,
// dot separated. A version therefore holds no dot.
func (s Sealed) Encode() string {
	enc := base64.RawURLEncoding
	return s.Version + "." + enc.EncodeToString(s.Salt) + "." + enc.EncodeToString(s.IV) + "." + enc.EncodeToString(s.Ciphertext)
}

// DecodeSealed reads what Encode wrote.
func DecodeSealed(s string) (Sealed, error) {
	parts := strings.SplitN(s, ".", 4)
	if len(parts) != 4 {
		return Sealed{}, errors.New("vault: not an encoded secret")
	}

	enc := base64.RawURLEncoding
	salt, err1 := enc.DecodeString(parts[1])
	iv, err2 := enc.DecodeString(parts[2])
	ct, err3 := enc.DecodeString(parts[3])
	if err := errors.Join(err1, err2, err3); err != nil {
		return Sealed{}, fmt.Errorf("vault: not an encoded secret: %w", err)
	}

	// GCM panics on a nonce of another size, and the value may come from
	// a cookie or a crafted row.
	if len(salt) != saltSize || len(iv) != ivSize || len(ct) < tagSize {
		return Sealed{}, errors.New("vault: not an encoded secret")
	}

	return Sealed{Version: parts[0], Salt: salt, IV: iv, Ciphertext: ct}, nil
}

// Derive is a 32 byte key for one purpose, named by info, from the
// master key. The same on every node holding the same master key,
// which is what a signed value crossing nodes needs.
func (v *Vault) Derive(info string) ([]byte, error) {
	master, err := v.key()
	if err != nil {
		return nil, err
	}

	key, err := hkdf.Key(sha256.New, master, nil, info, keySize)
	if err != nil {
		return nil, fmt.Errorf("vault: derive %s: %w", info, err)
	}

	return key, nil
}

// Open decrypts a secret stored for ownerID.
func (v *Vault) Open(ownerID string, s Sealed) (string, error) {
	if s.Version != KeyVersion {
		return "", fmt.Errorf("%w: %q", ErrUnknownVersion, s.Version)
	}

	gcm, err := v.aead(s.Salt, ownerID)
	if err != nil {
		return "", err
	}

	plain, err := gcm.Open(nil, s.IV, s.Ciphertext, []byte(ownerID))
	if err != nil {
		return "", fmt.Errorf("vault: open secret: %w", err)
	}

	return string(plain), nil
}

func (v *Vault) aead(salt []byte, ownerID string) (cipher.AEAD, error) {
	master, err := v.key()
	if err != nil {
		return nil, err
	}

	dataKey, err := hkdf.Key(sha256.New, master, salt, ownerID, keySize)
	if err != nil {
		return nil, fmt.Errorf("vault: derive key: %w", err)
	}

	block, err := aes.NewCipher(dataKey)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// ParseKey reads a master key: base64, a "base64:" prefix allowed.
func ParseKey(s string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(s), "base64:"))
	if err != nil {
		return nil, fmt.Errorf("vault: master key is not base64: %w", err)
	}

	if len(raw) != keySize {
		return nil, fmt.Errorf("vault: master key is %d bytes, want %d", len(raw), keySize)
	}

	return raw, nil
}

// TextPrefix marks a value sealed by SealText.
const TextPrefix = "gs1."

// SealText seals plain for a column, aad naming where it is stored
// (table, column and row), so a value moved elsewhere does not open.
// A nil vault stores clear text, which only tests use.
func (v *Vault) SealText(aad, plain string) (string, error) {
	if v == nil {
		return plain, nil
	}

	s, err := v.Seal(aad, plain)
	if err != nil {
		return "", err
	}

	return TextPrefix + s.Encode(), nil
}

// ErrNotSealed is a stored value that is not sealed text.
var ErrNotSealed = errors.New("vault: the stored value is not sealed")

// OpenText reads what SealText wrote. An empty value, a column's
// default, comes back empty, anything else unsealed is refused.
func (v *Vault) OpenText(aad, stored string) (string, error) {
	if v == nil || stored == "" {
		return stored, nil
	}

	rest, ok := strings.CutPrefix(stored, TextPrefix)
	if !ok {
		return "", ErrNotSealed
	}

	s, err := DecodeSealed(rest)
	if err != nil {
		return "", err
	}

	return v.Open(aad, s)
}

// IsText reports whether stored was written by SealText.
func IsText(stored string) bool {
	return strings.HasPrefix(stored, TextPrefix)
}

// Index is a keyed hash of value for one purpose, hex encoded: equal
// values give equal indexes, so a sealed column can still be matched,
// and nobody without the master key can compute one. A nil vault
// answers an unkeyed sha256.
func (v *Vault) Index(purpose, value string) (string, error) {
	if v == nil {
		sum := sha256.Sum256([]byte(purpose + "\x00" + value))

		return hex.EncodeToString(sum[:]), nil
	}

	key, err := v.Derive(purpose)
	if err != nil {
		return "", err
	}

	m := hmac.New(sha256.New, key)
	m.Write([]byte(value))

	return hex.EncodeToString(m.Sum(nil)), nil
}
