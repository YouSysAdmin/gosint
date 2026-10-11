// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/domain/user"
	prmodel "github.com/yousysadmin/gosint/internal/models/passwordreset"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// The mail budget: maxResetsPerHour links from one address for one
// account, maxResetsPerAccount over every address, or anyone knowing
// an address can flood the mailbox from an open endpoint.
const (
	maxResetsPerHour    = 3
	maxResetsPerAccount = 15

	// resetFloor is the least time a reset request takes, so an address
	// with an account costs no more than one without.
	resetFloor = 500 * time.Millisecond

	resetRefused = "this reset link is invalid or has expired, request a new one"
)

// PasswordResetRequest mails a single-use reset link. The answer is the
// same whether or not the address has an account, is disabled or is an
// identity provider's, and whether or not mail is configured: an open
// endpoint telling them apart is an enumeration oracle.
func (h Handler) PasswordResetRequest(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[resetRequestInput](c)
	if !ok {
		return resp
	}

	start := time.Now()
	accepted := func() error {
		if wait := resetFloor - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}

		return response.Success(c, messageResponse{Message: "If that address has an account, a reset link is on its way."})
	}

	u, err := h.Runtime.Store.User.GetByEmail(c, in.Email)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || !u.Active || !u.Local() {
		return accepted()
	}

	ip := c.IP()
	ips, err := h.Runtime.Store.PasswordReset.RecentRequestIPs(c, u.ID, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		return response.Internal(c, err)
	}

	if budgetSpent(ips, ip, maxResetsPerHour, maxResetsPerAccount) {
		h.Runtime.Log.Warn("auth: password reset throttled", "user", u.ID, "ip", ip)

		return accepted()
	}

	link, err := user.IssueReset(c, h.Runtime, u, ip)
	if err != nil {
		return response.Internal(c, err)
	}

	if !h.Runtime.Mail.Enabled() {
		h.Runtime.Log.Info("auth: password reset requested with no mail configured, an administrator can hand the link over", "user", u.ID)

		return accepted()
	}

	subject, htmlBody, textBody := mailer.PasswordReset(link, int(prmodel.TTL.Minutes()))
	mailer.SendAsync(h.Runtime.Mail, h.Runtime.Log, []string{u.Email}, subject, htmlBody, textBody)

	return accepted()
}

// PasswordResetConfirm redeems a token and sets the new password. The
// token is burned BEFORE anything changes: the read above is no guard,
// the conditional update is, and it only counts if the loser stops.
func (h Handler) PasswordResetConfirm(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[resetConfirmInput](c)
	if !ok {
		return resp
	}

	tok, err := h.Runtime.Store.PasswordReset.GetByHash(c, prmodel.Hash(in.Token))
	if err != nil {
		return response.Internal(c, err)
	}

	now := time.Now().UTC()
	if tok == nil || !tok.Usable(now) {
		return response.BadRequest(c, resetRefused)
	}

	u, err := h.Runtime.Store.User.Get(c, tok.UserID)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || !u.Active {
		return response.BadRequest(c, resetRefused)
	}

	claimed, err := h.Runtime.Store.PasswordReset.MarkUsed(c, tok.ID, now)
	if err != nil {
		return response.Internal(c, err)
	}

	if !claimed {
		return response.BadRequest(c, resetRefused)
	}

	if err := h.setPassword(c, u.ID, in.Password, now); err != nil {
		return response.Internal(c, err)
	}

	// Whoever prompted the reset may hold a live cookie.
	if _, err := h.Runtime.Store.Session.DeleteForUser(c, u.ID); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, messageResponse{Message: "Password updated. You can sign in with it now."})
}

// ChangePassword is the signed-in change: prove the current password,
// set a new one, end the other sessions and keep this one.
func (h Handler) ChangePassword(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[changePasswordInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.localSelf(c)
	if !ok {
		return resp
	}

	if refusal, refused := h.refuseReauth(c, u, in.CurrentPassword); refused {
		return refusal
	}

	now := time.Now().UTC()
	if err := h.setPassword(c, u.ID, in.Password, now); err != nil {
		return response.Internal(c, err)
	}

	if _, err := h.Runtime.Store.Session.DeleteOthers(c, u.ID, access.Session(c).ID); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, messageResponse{Message: "Password updated. Other sessions have been signed out."})
}

// setPassword hashes and stores a password and burns every outstanding
// reset link, which would otherwise set it again.
func (h Handler) setPassword(ctx context.Context, userID, password string, now time.Time) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), user.BcryptCost)
	if err != nil {
		return err
	}

	if err := h.Runtime.Store.User.SetPassword(ctx, userID, string(hash), now); err != nil {
		return err
	}

	return h.Runtime.Store.PasswordReset.InvalidateForUser(ctx, userID, now)
}

// localSelf is the signed-in account read fresh, refused when an
// identity provider owns its sign-in.
func (h Handler) localSelf(c fiber.Ctx) (*usermodel.User, error, bool) {
	u, err := h.Runtime.Store.User.Get(c, access.User(c).ID)
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if u == nil || !u.Active {
		return nil, response.Unauthorized(c, "authentication required"), false
	}

	if !u.Local() {
		return nil, response.Forbidden(c, "this account signs in through an identity provider, manage its sign-in there"), false
	}

	return u, nil, true
}

// budgetSpent decides whether one more link may be issued: the calling
// address has its own budget, the account a larger one over all.
func budgetSpent(ips []string, caller string, perIP, perAccount int) bool {
	if len(ips) >= perAccount {
		return true
	}

	key, fromCaller := AddressKey(caller), 0
	for _, ip := range ips {
		if AddressKey(ip) == key {
			fromCaller++
		}
	}

	return fromCaller >= perIP
}

// profileOf is the owner's profile for a sign-in answer.
func (h Handler) profileOf(ctx context.Context, u *usermodel.User) (user.Profile, error) {
	return user.ProfileFor(ctx, h.Runtime.Store, u)
}
