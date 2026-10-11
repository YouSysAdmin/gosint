// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// Recovery codes, the way back in when the authenticator is gone. Ten
// codes of sixteen symbols from an alphabet without the letters that
// read as digits, 31^16 is about 2^79, stored under a plain sha256
// since the input is random and long. Ported from mailyard.
const (
	recoveryCodeCount = 10
	recoveryAlphabet  = "abcdefghjkmnpqrstuvwxyz23456789"
	recoveryCodeLen   = 16
	recoveryGroup     = 4
)

// newRecoveryCodes mints a set: the codes as the person sees them
// (xxxx-xxxx-xxxx-xxxx) beside the hashes that are stored.
func newRecoveryCodes() (codes, hashes []string, err error) {
	codes = make([]string, 0, recoveryCodeCount)
	hashes = make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		var b strings.Builder
		n := 0
		for n < recoveryCodeLen {
			// One byte at a time through the sampler, rejection rather
			// than modulo, so no symbol gets an extra chance.
			var r [1]byte
			if _, err := rand.Read(r[:]); err != nil {
				return nil, nil, err
			}

			sym, ok := recoverySymbol(r[0])
			if !ok {
				continue
			}

			if n > 0 && n%recoveryGroup == 0 {
				b.WriteByte('-')
			}

			b.WriteByte(sym)
			n++
		}

		codes = append(codes, b.String())
		hashes = append(hashes, hashRecoveryCode(b.String()))
	}

	return codes, hashes, nil
}

// recoverySymbol maps one random byte onto the alphabet without bias,
// rejecting the tail that would wrap unevenly.
func recoverySymbol(r byte) (byte, bool) {
	const fair = 256 - 256%len(recoveryAlphabet)
	if int(r) >= fair {
		return 0, false
	}

	return recoveryAlphabet[int(r)%len(recoveryAlphabet)], true
}

// normalizeRecoveryCode folds what a person typed, case, dashes and
// spaces, to the symbols that identify the code.
func normalizeRecoveryCode(in string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(in)))
}

// looksLikeRecoveryCode tells a recovery code from an authenticator
// code at sign-in, where both arrive in one field.
func looksLikeRecoveryCode(in string) bool {
	n := normalizeRecoveryCode(in)
	if len(n) != recoveryCodeLen {
		return false
	}

	for i := range len(n) {
		if !strings.ContainsRune(recoveryAlphabet, rune(n[i])) {
			return false
		}
	}

	return true
}

// hashRecoveryCode is the stored form.
func hashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))

	return hex.EncodeToString(sum[:])
}

// issueRecoveryCodes mints and stores a fresh set, returning the codes
// to show once.
func (h Handler) issueRecoveryCodes(ctx context.Context, userID string) ([]string, error) {
	codes, hashes, err := newRecoveryCodes()
	if err != nil {
		return nil, err
	}

	if err := h.Runtime.Store.User.ReplaceRecoveryCodes(ctx, userID, hashes, time.Now().UTC()); err != nil {
		return nil, err
	}

	return codes, nil
}

// consumeSecondFactor is the sign-in check: an authenticator code, or
// failing that a recovery code, under ONE lock.
func (h Handler) consumeSecondFactor(c fiber.Ctx, u *usermodel.User, code string) bool {
	if h.locked(c, u.ID, "totp") {
		return false
	}

	if step, ok := h.verifyTOTP(u.ID, u.TOTPSecret, code); ok {
		return h.claimStep(c, u.ID, step)
	}

	if looksLikeRecoveryCode(code) {
		claimed, err := h.Runtime.Store.User.ClaimRecoveryCode(c, u.ID, hashRecoveryCode(code), time.Now().UTC())
		if err != nil {
			h.Runtime.Log.Error("auth: recovery code claim failed", "user", u.ID, "err", err)

			return false
		}

		if claimed {
			h.clearFailures(c, u.ID, "totp")
			remaining, _ := h.Runtime.Store.User.RemainingRecoveryCodes(c, u.ID)
			h.Runtime.Log.Warn("auth: a recovery code signed in", "user", u.ID, "remaining", remaining)

			return true
		}
	}

	h.recordFailure(c, u.ID, "totp", totpMaxFailures, totpLockout)

	return false
}

// RecoveryCodesStatus answers how many unspent codes the account holds.
func (h Handler) RecoveryCodesStatus(c fiber.Ctx) error {
	u, resp, ok := h.localSelf(c)
	if !ok {
		return resp
	}

	remaining, err := h.Runtime.Store.User.RemainingRecoveryCodes(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, recoveryCodesStatusResponse{Remaining: remaining})
}

// RecoveryCodesRegenerate mints a fresh set, voiding the old one,
// behind the password: the person asking usually lost the
// authenticator, and a hijacked session must not mint itself a way
// back in.
func (h Handler) RecoveryCodesRegenerate(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[reauthInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.localSelf(c)
	if !ok {
		return resp
	}

	if !u.TOTPEnabled {
		return response.BadRequest(c, "two-factor authentication is not on")
	}

	if refusal, refused := h.refuseReauth(c, u, in.Password); refused {
		return refusal
	}

	codes, err := h.issueRecoveryCodes(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, recoveryCodesResponse{Codes: codes})
}
