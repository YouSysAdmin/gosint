// Gosint, Copyright (c) 2026 YouSysAdmin

// Package envelope seals an export under a passphrase, so a file that
// leaves the server can only be opened by someone who knows it. The key
// comes from the passphrase through Argon2id, the data is AES-256-GCM.
// The vault's master key is not used: an export moves between installs
// and the other side has no key of ours.
package envelope

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json/v2"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Kind marks a sealed file, so an import tells it from a plain one.
const Kind = "encrypted"

// ErrPassphrase is a wrong passphrase, or a file someone changed.
var ErrPassphrase = errors.New("the passphrase does not open this file")

// ErrNotSealed is a file that is not an envelope.
var ErrNotSealed = errors.New("the file is not an encrypted export")

// ErrBusy is a key derivation refused because enough already run: each
// holds 64 MB, and the count is what bounds the memory they take.
var ErrBusy = errors.New("the server is busy deriving keys, try again shortly")

// kdfSlots bounds the Argon2id runs of the whole process, the unseal
// and the exports alike. kdfWait is how long a call waits for a slot.
var kdfSlots = make(chan struct{}, 2)

const kdfWait = 10 * time.Second

// Argon2id costs, the RFC 9106 second recommendation: 64 MB, three
// passes, which an import or an export pays once.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
	keySize      = 32
	saltSize     = 16
)

// file is the envelope on disk.
type file struct {
	Gosint  string `json:"gosint"`
	KDF     string `json:"kdf"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"`
	Threads uint8  `json:"threads"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

// Seal encrypts plaintext under the passphrase and returns the envelope
// as JSON.
func Seal(passphrase string, plaintext []byte) ([]byte, error) {
	return SealKind(Kind, passphrase, plaintext)
}

// SealKind is Seal for another use than an export: kind is written in
// the envelope and bound as its AAD, so one kind never opens as another.
func SealKind(kind, passphrase string, plaintext []byte) ([]byte, error) {
	if strings.TrimSpace(passphrase) == "" {
		return nil, errors.New("a passphrase is required")
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	f := file{Gosint: kind, KDF: "argon2id", Time: argonTime, Memory: argonMemory, Threads: argonThreads, Salt: salt}
	gcm, err := aead(passphrase, f)
	if err != nil {
		return nil, err
	}

	f.Nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(f.Nonce); err != nil {
		return nil, err
	}

	f.Data = gcm.Seal(nil, f.Nonce, plaintext, []byte(kind))

	return json.Marshal(f)
}

// IsSealed reports whether data is an envelope.
func IsSealed(data []byte) bool {
	var head struct {
		Gosint string `json:"gosint"`
	}

	return json.Unmarshal(data, &head) == nil && head.Gosint == Kind
}

// Open decrypts an envelope with the passphrase.
func Open(passphrase string, data []byte) ([]byte, error) {
	return OpenKind(Kind, passphrase, data)
}

// OpenKind is Open for an envelope written by SealKind with kind.
func OpenKind(kind, passphrase string, data []byte) ([]byte, error) {
	var f file
	if err := json.Unmarshal(data, &f); err != nil || f.Gosint != kind {
		return nil, ErrNotSealed
	}

	// The costs are the file's, bounded by what Seal writes, so a
	// crafted file costs an import no more than a real one does.
	if f.KDF != "argon2id" || f.Time == 0 || f.Time > argonTime || f.Memory < 8*1024 || f.Memory > argonMemory ||
		f.Threads == 0 || f.Threads > argonThreads || len(f.Salt) < 8 || len(f.Salt) > 64 {
		return nil, ErrNotSealed
	}

	gcm, err := aead(passphrase, f)
	if err != nil {
		return nil, err
	}

	if len(f.Nonce) != gcm.NonceSize() {
		return nil, ErrNotSealed
	}

	out, err := gcm.Open(nil, f.Nonce, f.Data, []byte(kind))
	if err != nil {
		return nil, ErrPassphrase
	}

	return out, nil
}

// aead derives the key the envelope's parameters say and opens the cipher.
func aead(passphrase string, f file) (cipher.AEAD, error) {
	select {
	case kdfSlots <- struct{}{}:
	case <-time.After(kdfWait):
		return nil, ErrBusy
	}

	key := argon2.IDKey([]byte(passphrase), f.Salt, f.Time, f.Memory, f.Threads, keySize)
	<-kdfSlots

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}
