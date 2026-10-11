// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"time"

	"github.com/yousysadmin/gosint/internal/database"

	"github.com/gofiber/fiber/v3"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/core/vault"
)

// The authenticator: period and skew are pquerna/otp's defaults, which
// totp.Generate mints against. Five wrong codes lock the factor for
// fifteen minutes, more than a person mistypes and a thousandth of
// what a guess at six digits needs.
const (
	totpPeriod      = 30
	totpSkew        = 1
	totpMaxFailures = 5
	totpLockout     = 15 * time.Minute

	// twoFactorCookie carries who passed the password while the code
	// is asked, sealed, for twoFactorTTL.
	twoFactorCookie = "gosint_2fa"
	twoFactorTTL    = 5 * time.Minute
)

// pendingSecondFactor is the sealed state between the password and the
// code.
type pendingSecondFactor struct {
	UserID string `json:"user_id"`

	// Invite is the invitation an identity provider sign-in started
	// from, handed back so the console lands on it.
	Invite string `json:"invite,omitzero"`
}

// TOTPSetup mints a fresh secret and stores it sealed, not yet enabled.
// Enrolling is gated by the password, as passkey enrolment is, so a
// hijacked session cannot enrol a factor the owner does not hold.
func (h Handler) TOTPSetup(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[reauthInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.localSelf(c)
	if !ok {
		return resp
	}

	if u.TOTPEnabled {
		return response.Conflict(c, "two-factor authentication is already on, turn it off first")
	}

	if refusal, refused := h.refuseReauth(c, u, in.Password); refused {
		return refusal
	}

	key, err := totp.Generate(totp.GenerateOpts{Issuer: "Gosint", AccountName: u.Email})
	if err != nil {
		return response.Internal(c, err)
	}

	sealed, err := h.Runtime.Vault.SealText(totpAAD(u.ID), key.Secret())
	if err != nil {
		return response.Internal(c, err)
	}

	if err := h.Runtime.Store.User.SetTOTP(c, u.ID, sealed, false); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, totpSetupResponse{Secret: key.Secret(), OTPAuthURL: key.URL()})
}

// TOTPEnable turns the factor on once a code proves the authenticator
// holds the pending secret, ends the other sessions and hands out the
// recovery codes, shown once.
func (h Handler) TOTPEnable(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[totpCodeInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.localSelf(c)
	if !ok {
		return resp
	}

	if u.TOTPSecret == "" {
		return response.BadRequest(c, "set the authenticator up first")
	}

	if !h.consumeTOTP(c, u.ID, u.TOTPSecret, in.Code) {
		return response.BadRequest(c, "the code was not accepted")
	}

	if err := h.Runtime.Store.User.SetTOTP(c, u.ID, u.TOTPSecret, true); err != nil {
		return response.Internal(c, err)
	}

	h.endOtherSessions(c, u.ID)
	codes, err := h.issueRecoveryCodes(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, totpStateResponse{TOTPEnabled: true, RecoveryCodes: codes})
}

// TOTPDisable turns the factor off behind a current code, so a hijacked
// session cannot quietly weaken the account. The recovery codes go with
// it.
func (h Handler) TOTPDisable(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[totpCodeInput](c)
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

	if !h.consumeTOTP(c, u.ID, u.TOTPSecret, in.Code) {
		return response.BadRequest(c, "the code was not accepted")
	}

	if err := h.Runtime.Store.User.SetTOTP(c, u.ID, "", false); err != nil {
		return response.Internal(c, err)
	}

	if err := h.Runtime.Store.User.DeleteRecoveryCodes(c, u.ID); err != nil {
		return response.Internal(c, err)
	}

	h.endOtherSessions(c, u.ID)

	return response.Success(c, totpStateResponse{TOTPEnabled: false})
}

// LoginSecondFactor is the second step of a sign-in: the sealed cookie
// says who passed the password, the code or a recovery code opens the
// session. A wrong code keeps the cookie, the factor lock bounds the
// guesses.
func (h Handler) LoginSecondFactor(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[secondFactorInput](c)
	if !ok {
		return resp
	}

	pending, err := openCookie[pendingSecondFactor](c, h.Runtime, twoFactorCookie)
	if err != nil {
		return response.Unauthorized(c, "sign in with your password first")
	}

	u, err := h.Runtime.Store.User.Get(c, pending.UserID)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || !u.Active || !u.TOTPEnabled {
		clearNamed(c, h.Runtime, twoFactorCookie)

		return response.Unauthorized(c, "sign in with your password first")
	}

	if !h.consumeSecondFactor(c, u, in.Code) {
		return response.Unauthorized(c, "the code was not accepted")
	}

	clearNamed(c, h.Runtime, twoFactorCookie)
	if err := startSession(c, h.Runtime, u); err != nil {
		return response.Internal(c, err)
	}

	p, err := h.profileOf(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, loginResponse{User: &p, Invite: pending.Invite})
}

// verifyTOTP opens the sealed secret and validates the code, returning
// the time step the code belongs to. The step matters because a code
// must work once: with skew 1 three codes are live at any instant, so
// one stays valid for up to 90 seconds. pquerna/otp only answers yes
// or no, so the step is found by asking about each candidate with the
// skew off.
func (h Handler) verifyTOTP(userID, stored, code string) (int64, bool) {
	// Never read as clear text.
	if !vault.IsText(stored) {
		return 0, false
	}

	secret, err := h.Runtime.Vault.OpenText(totpAAD(userID), stored)
	if err != nil {
		return 0, false
	}

	now := time.Now()
	opts := totp.ValidateOpts{Period: totpPeriod, Skew: 0, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1}
	for delta := -totpSkew; delta <= totpSkew; delta++ {
		at := now.Add(time.Duration(delta*totpPeriod) * time.Second)
		valid, verr := totp.ValidateCustom(code, secret, at, opts)
		if verr == nil && valid && at.Unix() >= 0 {
			return at.Unix() / totpPeriod, true
		}
	}

	return 0, false
}

// consumeTOTP validates a code and burns its step, so the same code
// cannot be presented twice. Every check goes through here or through
// consumeSecondFactor, never verifyTOTP alone.
func (h Handler) consumeTOTP(ctx context.Context, userID, encoded, code string) bool {
	if h.locked(ctx, userID, "totp") {
		return false
	}

	step, ok := h.verifyTOTP(userID, encoded, code)
	if !ok {
		h.recordFailure(ctx, userID, "totp", totpMaxFailures, totpLockout)

		return false
	}

	return h.claimStep(ctx, userID, step)
}

// claimStep burns a verified step and, on success, the failure count.
func (h Handler) claimStep(ctx context.Context, userID string, step int64) bool {
	claimed, err := h.Runtime.Store.User.ClaimTOTPStep(ctx, userID, step)
	if err != nil {
		h.Runtime.Log.Error("auth: totp step claim failed", "user", userID, "err", err)

		return false
	}

	if !claimed {
		h.Runtime.Log.Warn("auth: totp code replayed", "user", userID, "step", step)

		return false
	}

	h.clearFailures(ctx, userID, "totp")

	return true
}

// endOtherSessions ends every session of the account but this one: a
// second factor changes in answer to a session the owner did not start.
func (h Handler) endOtherSessions(c fiber.Ctx, userID string) {
	if _, err := h.Runtime.Store.Session.DeleteOthers(c, userID, access.Session(c).ID); err != nil {
		h.Runtime.Log.Warn("auth: ending the other sessions failed", "user", userID, "err", err)
	}
}

// totpAAD binds a sealed TOTP secret to its account's column.
func totpAAD(userID string) string { return database.AAD("users", "totp_secret", userID) }
