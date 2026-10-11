// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/user"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// dummyHash is checked against when the email is unknown, so a login
// takes as long whether or not the account exists. Made on first use,
// at the cost hashes are made with.
var dummyHash = sync.OnceValue(func() []byte {
	h, _ := bcrypt.GenerateFromPassword([]byte("gosint-timing-equalizer"), user.BcryptCost)

	return h
})

const badCredentials = "incorrect email or password"

// Handler serves registration, login and logout.
type Handler struct {
	Runtime *env.Runtime
}

// Register creates an account and signs it in.
func (h Handler) Register(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[registerInput](c)
	if !ok {
		return resp
	}

	// The door is checked before the hash: on a closed install a
	// refusal costs no bcrypt, which an anonymous caller could
	// otherwise spend at will.
	open, invited := h.Runtime.Config.Auth.OpenRegistration, false
	if !open {
		n, err := h.Runtime.Store.User.Count(c)
		if err != nil {
			return response.Internal(c, err)
		}

		if n > 0 {
			if invited, err = h.invited(c, in.Invite, in.Email); err != nil {
				return response.Internal(c, err)
			}

			if !invited {
				return response.Forbidden(c, "registration is closed")
			}
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), user.BcryptCost)
	if err != nil {
		return response.Internal(c, err)
	}

	now := time.Now().UTC()
	u := &usermodel.User{
		ID:           ids.New(),
		Email:        in.Email,
		PasswordHash: string(hash),
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// The first account claims the install and is its administrator,
	// counted and inserted as one so two people cannot both get in.
	// After it, registration is open or closed by the config.
	created, err := h.Runtime.Store.User.CreateFirst(c, u)
	if err == nil && !created {
		// Someone claimed the install since the check above: a pending
		// invitation for this address still opens the door for it.
		if !open && !invited {
			ok, ierr := h.invited(c, in.Invite, in.Email)
			if ierr != nil {
				return response.Internal(c, ierr)
			}

			if !ok {
				return response.Forbidden(c, "registration is closed")
			}
		}

		err = h.Runtime.Store.User.Create(c, u)
	}

	if database.UniqueViolation(err) {
		return response.Conflict(c, "email already registered")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	if err := startSession(c, h.Runtime, u); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, sessionResponse{User: user.ProfileOf(u)})
}

// invited says token is a pending invitation issued to email.
func (h Handler) invited(ctx context.Context, token, email string) (bool, error) {
	if token == "" {
		return false, nil
	}

	inv, err := h.Runtime.Store.Investigation.PendingInvitationFor(ctx, invmodel.HashInvitationToken(token), email, time.Now().UTC())

	return inv != nil, err
}

// Login checks a password and signs the account in.
func (h Handler) Login(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[loginInput](c)
	if !ok {
		return resp
	}

	// Every refusal answers no sooner than the floor after this, so the
	// legs cannot be told apart by timing.
	start := time.Now()
	u, err := h.Runtime.Store.User.GetByEmail(c, in.Email)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || !u.Active {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(in.Password))

		return refuseLogin(c, start)
	}

	// An identity provider's account has no password here, its empty
	// hash fails the check like a wrong password.
	if ok, _ := h.checkPassword(c, u, in.Password); !ok {
		return refuseLogin(c, start)
	}

	// The second factor. The password checked out, so saying a code is
	// owed tells the holder of valid credentials nothing new.
	if u.TOTPEnabled {
		if err := sealCookie(c, h.Runtime, twoFactorCookie, pendingSecondFactor{UserID: u.ID}, twoFactorTTL); err != nil {
			return response.Internal(c, err)
		}

		return response.Success(c, loginResponse{SecondFactor: "totp"})
	}

	if err := startSession(c, h.Runtime, u); err != nil {
		return response.Internal(c, err)
	}

	p, err := h.profileOf(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, loginResponse{User: &p})
}

// Info tells the sign-in page what this install offers. Public, it
// names no account.
func (h Handler) Info(c fiber.Ctx) error {
	n, err := h.Runtime.Store.User.Count(c)
	if err != nil {
		return response.Internal(c, err)
	}

	open := h.Runtime.Config.Auth.OpenRegistration || n == 0

	providers, err := h.loginProviders(c)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, authInfoResponse{PasskeysEnabled: h.Runtime.Config.Auth.PasskeysEnabled,
		MailEnabled: h.Runtime.Mail.Enabled(), RegistrationOpen: open, SetupRequired: n == 0, Providers: providers})
}

// Logout ends the current session.
func (h Handler) Logout(c fiber.Ctx) error {
	if token := c.Cookies(CookieName); token != "" {
		if err := h.Runtime.Store.Session.Delete(c, hashToken(token)); err != nil {
			return response.Internal(c, err)
		}
	}

	clearCookie(c, h.Runtime)

	return response.NoContent(c)
}
