// Gosint, Copyright (c) 2026 YouSysAdmin

package sys

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/yousysadmin/gosint/internal/core/vault"
)

// MinUnsealKey is the shortest unseal key the seal takes, in characters.
const MinUnsealKey = 12

// checkAAD and checkText make the key check: a value only the right
// master key opens.
const (
	checkAAD  = "vault_seal.key_check"
	checkText = "gosint"
)

var (
	// ErrNotInitialized is a server with no wrapped key yet.
	ErrNotInitialized = errors.New("the server has no master key yet, initialize it")

	// ErrInitialized is a second initialization.
	ErrInitialized = errors.New("the server is initialized already")

	// ErrShortUnsealKey is an unseal key under MinUnsealKey.
	ErrShortUnsealKey = fmt.Errorf("the unseal key needs %d characters or more", MinUnsealKey)

	// ErrKeyMismatch is a configured key the data is not sealed under.
	ErrKeyMismatch = errors.New("vault.master_key is not the key this database is sealed under")

	// ErrKeyLost is a database holding sealed data and no wrapped key:
	// a fresh key would leave the data unread.
	ErrKeyLost = errors.New("the database holds sealed data but no key for it, restore the vault_seal row")
)

// Initialize wraps a master key under unsealKey and stores it, and
// leaves v unsealed with it: the key v already holds (the configured
// one in development), a fresh one otherwise.
func Initialize(ctx context.Context, st *Store, v *vault.Vault, unsealKey string) error {
	if utf8.RuneCountInString(unsealKey) < MinUnsealKey {
		return ErrShortUnsealKey
	}

	// An initialized server is refused before the key derivation, which
	// costs 64 MB.
	if row, err := st.Get(ctx); err != nil {
		return err
	} else if row != nil {
		return ErrInitialized
	}

	// The key is wrapped from a vault of its own, and v only takes a
	// fresh one once the row is this call's.
	keys, fresh := v, []byte(nil)
	if v.IsSealed() {
		if sealed, err := st.HoldsSealedData(ctx); err != nil {
			return err
		} else if sealed {
			return ErrKeyLost
		}

		fresh = vault.NewMaster()
		probe, err := vault.New(fresh)
		if err != nil {
			return err
		}

		keys = probe
	}

	check, err1 := keys.SealText(checkAAD, checkText)
	wrapped, err2 := keys.Wrap(unsealKey)
	if err := errors.Join(err1, err2); err != nil {
		return err
	}

	made, err := st.Create(ctx, Row{Wrapped: string(wrapped), KeyCheck: check}, time.Now().UTC())
	if err != nil {
		return err
	}

	if !made {
		return ErrInitialized
	}

	if fresh != nil {
		return v.Unseal(fresh)
	}

	return nil
}

// Unseal unwraps the master key with unsealKey into v.
func Unseal(ctx context.Context, st *Store, v *vault.Vault, unsealKey string) error {
	row, err := st.Get(ctx)
	if err != nil {
		return err
	}

	if row == nil {
		return ErrNotInitialized
	}

	master, err := vault.Unwrap(unsealKey, []byte(row.Wrapped))
	if err != nil {
		return err
	}

	return v.Unseal(master)
}

// Rekey wraps the master key under a new unseal key. The data is not
// touched, only the unseal key that opens it changes.
func Rekey(ctx context.Context, st *Store, oldUnsealKey, newUnsealKey string) error {
	if utf8.RuneCountInString(newUnsealKey) < MinUnsealKey {
		return ErrShortUnsealKey
	}

	row, err := st.Get(ctx)
	if err != nil {
		return err
	}

	if row == nil {
		return ErrNotInitialized
	}

	master, err := vault.Unwrap(oldUnsealKey, []byte(row.Wrapped))
	if err != nil {
		return err
	}

	wrapped, err := vault.Wrap(newUnsealKey, master)
	if err != nil {
		return err
	}

	return st.Rewrap(ctx, string(wrapped), time.Now().UTC())
}

// Matches reports whether master is the key the stored seal was made
// with, for a key given in the config.
func Matches(row *Row, master []byte) bool {
	probe, err := vault.New(master)
	if err != nil {
		return false
	}

	got, err := probe.OpenText(checkAAD, row.KeyCheck)

	return err == nil && got == checkText
}
