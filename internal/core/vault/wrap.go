// Gosint, Copyright (c) 2026 YouSysAdmin

package vault

import (
	"errors"

	"github.com/yousysadmin/gosint/internal/core/envelope"
)

// wrapKind marks a wrapped master key, so it never opens as an export
// and an export never as a key.
const wrapKind = "gosint-vault-key"

// ErrUnsealKey is an unseal key that does not unwrap the master key.
var ErrUnsealKey = errors.New("vault: the unseal key does not open this server")

// Wrap seals the master key under an unseal key (Argon2id, AES-256-GCM),
// the form it is stored in, so the database alone opens nothing.
func Wrap(unsealKey string, master []byte) ([]byte, error) {
	if len(master) != keySize {
		return nil, errors.New("vault: master key has the wrong size")
	}

	return envelope.SealKind(wrapKind, unsealKey, master)
}

// Wrap seals the vault's own master key under an unseal key.
func (v *Vault) Wrap(unsealKey string) ([]byte, error) {
	master, err := v.key()
	if err != nil {
		return nil, err
	}

	return Wrap(unsealKey, master)
}

// Unwrap reads the master key back with the unseal key.
func Unwrap(unsealKey string, wrapped []byte) ([]byte, error) {
	master, err := envelope.OpenKind(wrapKind, unsealKey, wrapped)
	if errors.Is(err, envelope.ErrPassphrase) {
		return nil, ErrUnsealKey
	}

	if err != nil {
		return nil, err
	}

	if len(master) != keySize {
		return nil, errors.New("vault: the stored master key has the wrong size")
	}

	return master, nil
}
