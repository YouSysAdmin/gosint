// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"strings"
	"testing"
)

// A set is ten distinct codes in the spelling a person can copy from a
// printout, and the hash forgives how they typed it back.
func TestRecoveryCodesAreDistinctAndForgivinglyHashed(t *testing.T) {
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}

	if len(codes) != recoveryCodeCount || len(hashes) != recoveryCodeCount {
		t.Fatalf("got %d codes and %d hashes", len(codes), len(hashes))
	}

	seen := map[string]bool{}
	for i, c := range codes {
		groups := strings.Split(c, "-")
		if len(groups) != recoveryCodeLen/recoveryGroup {
			t.Errorf("code %q is not xxxx-xxxx-xxxx-xxxx", c)
		}

		if !looksLikeRecoveryCode(c) {
			t.Errorf("code %q does not look like one to the sign-in check", c)
		}

		if seen[c] {
			t.Errorf("code %q minted twice", c)
		}

		seen[c] = true
		typed := " " + strings.ToUpper(strings.ReplaceAll(c, "-", "")) + " "
		if hashRecoveryCode(typed) != hashes[i] {
			t.Errorf("hash of %q typed as %q differs", c, typed)
		}
	}

	for _, not := range []string{"123456", "abcde-fghjk-m", "abcd0-1234-abcd-abcd", ""} {
		if looksLikeRecoveryCode(not) {
			t.Errorf("%q was taken for a recovery code", not)
		}
	}
}

// Every symbol gets the same share of the byte space, which is what the
// rejection sampler is for.
func TestRecoverySymbolsAreUnbiased(t *testing.T) {
	counts := map[byte]int{}
	rejected := 0
	for r := range 256 {
		sym, ok := recoverySymbol(byte(r))
		if !ok {
			rejected++

			continue
		}

		counts[sym]++
	}

	if len(counts) != len(recoveryAlphabet) {
		t.Fatalf("%d symbols reachable, want %d", len(counts), len(recoveryAlphabet))
	}

	for sym, n := range counts {
		if n != 8 {
			t.Errorf("symbol %q has %d preimages, want 8", sym, n)
		}
	}

	if rejected != 256%len(recoveryAlphabet) {
		t.Errorf("%d bytes rejected, want %d", rejected, 256%len(recoveryAlphabet))
	}
}
