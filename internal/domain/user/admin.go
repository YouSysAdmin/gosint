// Gosint, Copyright (c) 2026 YouSysAdmin

package user

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/paging"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	prmodel "github.com/yousysadmin/gosint/internal/models/passwordreset"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// The admin panel, ported from mailyard. Every route is behind
// requireSession and requireAdmin, so what is left here is the self
// guard (an admin cannot lock themselves out) and the LAST ENABLED
// ADMINISTRATOR, who cannot be demoted, disabled or deleted.

// BcryptCost is the work factor of every password hash. Tests lower it
// to bcrypt.MinCost before anything runs, a server never changes it.
var BcryptCost = 12

// ErrLastAdmin is a change that would leave the install with nobody to
// administer it.
var ErrLastAdmin = errors.New("the last enabled administrator stays")

const lastAdminMessage = "this is the last enabled administrator, promote somebody else first"

// adminView is an account as the panel lists it.
func (h Handler) adminView(ctx context.Context, u *usermodel.User) (AdminUser, error) {
	p, err := ProfileFor(ctx, h.Runtime.Store, u)
	if err != nil {
		return AdminUser{}, err
	}

	return AdminUser{Profile: p, Active: u.Active, CreatedAt: u.CreatedAt}, nil
}

// AdminList answers the accounts, newest first, filtered by a search
// over the email and the two flags.
func (h Handler) AdminList(c fiber.Ctx) error {
	f := UserFilter{Query: strings.ToLower(strings.TrimSpace(c.Query("q"))), Limit: paging.Limit(c, 50, 500)}
	for _, p := range []struct {
		name string
		dst  **bool
	}{{"admin", &f.Admin}, {"disabled", &f.Disabled}} {
		switch c.Query(p.name) {
		case "":
		case "true":
			*p.dst = new(true)
		case "false":
			*p.dst = new(false)
		default:
			return response.BadRequest(c, p.name+" must be true or false")
		}
	}

	users, total, err := h.Runtime.Store.User.Find(c, f)
	if err != nil {
		return response.Internal(c, err)
	}

	out := adminUsersResponse{Users: make([]AdminUser, 0, len(users)), Total: total}
	for i := range users {
		v, err := h.adminView(c, &users[i])
		if err != nil {
			return response.Internal(c, err)
		}

		out.Users = append(out.Users, v)
	}

	return response.Success(c, out)
}

// AdminGet answers one account.
func (h Handler) AdminGet(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	v, err := h.adminView(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	owned, err := h.Runtime.Store.Investigation.OwnedBy(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, adminUserResponse{User: v, Owns: owned})
}

// AdminCreate adds a local account. A password is optional: without
// one the person gets a reset link.
func (h Handler) AdminCreate(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[adminCreateInput](c)
	if !ok {
		return resp
	}

	now := time.Now().UTC()
	u := &usermodel.User{ID: ids.New(), Email: in.Email, Active: true, Admin: in.Admin, AccountType: usermodel.AccountLocal,
		CreatedAt: now, UpdatedAt: now}
	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
		if err != nil {
			return response.Internal(c, err)
		}

		u.PasswordHash = string(hash)
	}

	err := h.Runtime.Store.User.Create(c, u)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "an account with this email exists")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	v, err := h.adminView(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, adminUserResponse{User: v, Owns: []invmodelInvestigation{}})
}

// AdminUpdate changes the flags or sets a password. The flags are read
// BEFORE the change, which is the fact the guarded write is chosen on.
// Disabling, or a new password, ends the account's sessions at once.
func (h Handler) AdminUpdate(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[adminUpdateInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	self := u.ID == access.User(c).ID
	if self && (in.Admin != nil || in.Disabled != nil) {
		return response.Forbidden(c, "change your own flags from another administrator's account")
	}

	// The profile asks for the current password, a hijacked session
	// must not get round it here.
	if self && in.Password != "" {
		return response.Forbidden(c, "change your own password from your profile, with the current one")
	}

	if in.Password != "" && !u.Local() {
		return response.BadRequest(c, "this account signs in through an identity provider and holds no password")
	}

	now := time.Now().UTC()
	if in.Admin != nil || in.Disabled != nil {
		admin, active := u.Admin, u.Active
		if in.Admin != nil {
			admin = *in.Admin
		}

		if in.Disabled != nil {
			active = !*in.Disabled
		}

		keep := !(u.Admin && u.Active && (!admin || !active))
		changed, err := h.Runtime.Store.User.UpdateFlags(c, u.ID, admin, active, keep, now)
		if err != nil {
			return response.Internal(c, err)
		}

		if !changed {
			return response.Conflict(c, lastAdminMessage)
		}

		u.Admin, u.Active = admin, active
		if !active {
			h.endSessions(c, u.ID)
		}
	}

	if in.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), BcryptCost)
		if err != nil {
			return response.Internal(c, err)
		}

		if err := h.Runtime.Store.User.SetPassword(c, u.ID, string(hash), now); err != nil {
			return response.Internal(c, err)
		}

		if err := h.Runtime.Store.PasswordReset.InvalidateForUser(c, u.ID, now); err != nil {
			return response.Internal(c, err)
		}

		h.endSessions(c, u.ID)
	}

	v, err := h.adminView(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, adminUserResponse{User: v, Owns: []invmodelInvestigation{}})
}

// AdminDelete removes an account. Refused for oneself, for the owner of
// an investigation (transfer or delete it first) and for the last
// administrator.
func (h Handler) AdminDelete(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	if u.ID == access.User(c).ID {
		return response.Forbidden(c, "delete your own account from another administrator's account")
	}

	owned, err := h.Runtime.Store.Investigation.OwnedBy(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if len(owned) > 0 {
		names := make([]string, 0, len(owned))
		for _, inv := range owned {
			names = append(names, inv.Name)
		}

		return response.Conflict(c, "this account owns "+strings.Join(names, ", ")+", hand them over or delete them first")
	}

	gone, err := h.Runtime.Store.User.DeleteKeepingAnAdmin(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if !gone {
		return response.Conflict(c, lastAdminMessage)
	}

	return response.NoContent(c)
}

// AdminResetLink mints a reset link for the account and hands it back,
// mailing it too when mail is configured. An identity provider's
// account gets one only when its provider is gone, and the reset then
// makes it local.
func (h Handler) AdminResetLink(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	if u.ID == access.User(c).ID {
		return response.Forbidden(c, "change your own password from your profile, with the current one")
	}

	if !u.Local() {
		return response.BadRequest(c, "this account signs in through an identity provider")
	}

	link, err := IssueReset(c, h.Runtime, u, c.IP())
	if err != nil {
		return response.Internal(c, err)
	}

	emailed := h.Runtime.Mail.Enabled()
	if emailed {
		subject, htmlBody, textBody := mailer.PasswordReset(link, int(prmodel.TTL.Minutes()))
		mailer.SendAsync(h.Runtime.Mail, h.Runtime.Log, []string{u.Email}, subject, htmlBody, textBody)
	}

	return response.Success(c, resetLinkResponse{Link: link, Emailed: emailed})
}

// AdminResetTOTP removes an account's second factor, for the person who
// lost the phone. Refused on oneself: the profile does that with a code.
func (h Handler) AdminResetTOTP(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	if u.ID == access.User(c).ID {
		return response.Forbidden(c, "turn your own second factor off from your profile, with a code")
	}

	if err := h.Runtime.Store.User.SetTOTP(c, u.ID, "", false); err != nil {
		return response.Internal(c, err)
	}

	if err := h.Runtime.Store.User.DeleteRecoveryCodes(c, u.ID); err != nil {
		return response.Internal(c, err)
	}

	h.endSessions(c, u.ID)

	return response.NoContent(c)
}

// AdminResetPasskeys removes every passkey of an account. Refused on
// oneself, the profile does that with the password.
func (h Handler) AdminResetPasskeys(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	if u.ID == access.User(c).ID {
		return response.Forbidden(c, "remove your own passkeys from your profile, with your password")
	}

	n, err := h.Runtime.Store.Passkey.DeleteAllForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	h.endSessions(c, u.ID)

	return response.Success(c, removedResponse{Removed: n})
}

// AdminRevokeSessions ends every session of an account. Allowed on
// oneself, signing out everywhere is a legitimate wish.
func (h Handler) AdminRevokeSessions(c fiber.Ctx) error {
	u, resp, ok := h.target(c)
	if !ok {
		return resp
	}

	n, err := h.Runtime.Store.Session.DeleteForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, revokedResponse{Revoked: n})
}

// target is the account in the path.
func (h Handler) target(c fiber.Ctx) (*usermodel.User, error, bool) {
	u, err := h.Runtime.Store.User.Get(c, c.Params("id"))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if u == nil {
		return nil, response.NotFound(c, "user not found"), false
	}

	return u, nil, true
}

// endSessions signs the account out everywhere.
func (h Handler) endSessions(ctx context.Context, userID string) {
	if _, err := h.Runtime.Store.Session.DeleteForUser(ctx, userID); err != nil {
		h.Runtime.Log.Warn("users: ending sessions failed", "user", userID, "err", err)
	}
}
