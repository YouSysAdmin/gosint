// Gosint, Copyright (c) 2026 YouSysAdmin

package vault_test

import (
	"encoding/base64"
	"errors"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/vault"
)

const owner = "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"

func seq(from, n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(from + i) //nolint:gosec // small test values
	}

	return b
}

func newVault(t *testing.T) *vault.Vault {
	t.Helper()

	v, err := vault.New(seq(0, 32))
	if err != nil {
		t.Fatal(err)
	}

	return v
}

// A fixed vector made with python cryptography (HKDF-SHA256 + AES-GCM,
// owner id as info and AAD) checks the construction against a second
// implementation.
func TestOpensAKnownVector(t *testing.T) {
	ct, _ := base64.StdEncoding.DecodeString("SH1awC59z6z4yaIzNsrqc1KAbPEeco8auwMYzQkRk08=")

	got, err := newVault(t).Open(owner, vault.Sealed{Ciphertext: ct, IV: seq(200, 12), Salt: seq(100, 16), Version: "V1"})
	if err != nil {
		t.Fatal(err)
	}

	if got != "whoxy-secret-123" {
		t.Errorf("opened %q", got)
	}
}

func TestASecretOnlyOpensForItsOwner(t *testing.T) {
	v := newVault(t)

	s, err := v.Seal(owner, "token")
	if err != nil {
		t.Fatal(err)
	}

	if got, err := v.Open(owner, s); err != nil || got != "token" {
		t.Fatalf("round trip = %q, %v", got, err)
	}

	if _, err := v.Open("someone-else", s); err == nil {
		t.Error("another account opened the secret")
	}

	s.Version = "V9"
	if _, err := v.Open(owner, s); !errors.Is(err, vault.ErrUnknownVersion) {
		t.Errorf("unknown version: %v", err)
	}
}

func TestASealedVaultOpensNothing(t *testing.T) {
	v := vault.NewSealed()
	if _, err := v.Seal(owner, "x"); !errors.Is(err, vault.ErrSealed) {
		t.Fatalf("sealed Seal: %v", err)
	}

	select {
	case <-v.Ready():
		t.Fatal("ready before unseal")
	default:
	}

	if err := v.Unseal(seq(0, 32)); err != nil {
		t.Fatal(err)
	}

	<-v.Ready()

	stored, err := v.SealText("nodes.properties:1", "secret")
	if err != nil {
		t.Fatal(err)
	}

	v.Reseal()
	if _, err := v.OpenText("nodes.properties:1", stored); !errors.Is(err, vault.ErrSealed) {
		t.Errorf("resealed OpenText: %v", err)
	}
}

func TestTextIsBoundToItsPlace(t *testing.T) {
	v := newVault(t)

	stored, err := v.SealText("nodes.properties:1", "secret")
	if err != nil {
		t.Fatal(err)
	}

	if !vault.IsText(stored) {
		t.Fatalf("no prefix: %q", stored)
	}

	if got, err := v.OpenText("nodes.properties:1", stored); err != nil || got != "secret" {
		t.Fatalf("round trip = %q, %v", got, err)
	}

	if _, err := v.OpenText("nodes.properties:2", stored); err == nil {
		t.Error("a value opened in another row")
	}

	if _, err := v.OpenText("x", "plain"); !errors.Is(err, vault.ErrNotSealed) {
		t.Errorf("clear text: %v", err)
	}

	if got, err := v.OpenText("x", ""); err != nil || got != "" {
		t.Errorf("empty = %q, %v", got, err)
	}
}

func TestIndexIsStableAndKeyed(t *testing.T) {
	a, _ := newVault(t).Index("p", "value")
	b, _ := newVault(t).Index("p", "value")
	other, _ := vault.New(seq(1, 32))
	c, _ := other.Index("p", "value")

	if a != b || a == c || a == "" {
		t.Errorf("index a=%s b=%s c=%s", a, b, c)
	}
}

func TestTheMasterKeyUnwrapsWithItsUnsealKey(t *testing.T) {
	master := vault.NewMaster()

	wrapped, err := vault.Wrap("correct horse", master)
	if err != nil {
		t.Fatal(err)
	}

	got, err := vault.Unwrap("correct horse", wrapped)
	if err != nil || string(got) != string(master) {
		t.Fatalf("unwrap = %v", err)
	}

	if _, err := vault.Unwrap("wrong", wrapped); !errors.Is(err, vault.ErrUnsealKey) {
		t.Errorf("wrong unseal key: %v", err)
	}
}

func TestParseKeyTakesABase64Prefix(t *testing.T) {
	if _, err := vault.ParseKey("base64:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="); err != nil {
		t.Error(err)
	}

	if _, err := vault.ParseKey("c2hvcnQ="); err == nil {
		t.Error("a short key was accepted")
	}
}

// A value with a nonce or salt of another size is refused, never handed
// to GCM, which panics on a nonce of the wrong size.
func TestAMalformedSealIsRefused(t *testing.T) {
	v := newVault(t)
	for _, stored := range []string{"gs1.V1.AAAA.AA.AAAA", "gs1.V1...", "gs1.V1.AAAAAAAAAAAAAAAAAAAAAA.AAAAAAAAAAAAAAAA.AA"} {
		if _, err := v.OpenText("x", stored); err == nil {
			t.Errorf("%q opened", stored)
		}
	}
}
