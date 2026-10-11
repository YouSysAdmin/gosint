// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"net/netip"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"

	"github.com/yousysadmin/gosint/internal/core/response"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// The account lock: loginMaxFailures wrong passwords in a row lock the
// account for loginLockout, refused before the password is looked at,
// so a right guess during the lock learns nothing. On top of the
// per-address limiter on the sign-in routes. Mailyard's per-address
// admissions during a lock are not ported, the limiter covers the
// stranger and the owner waits fifteen minutes.
const (
	loginMaxFailures = 5
	loginLockout     = 15 * time.Minute

	// loginFailureFloor is the least time a refused sign-in takes. A
	// known account does database work an unknown one does not, and the
	// floor hides the difference.
	loginFailureFloor = 750 * time.Millisecond
)

// refuseLogin answers a refused sign-in once the floor has passed.
func refuseLogin(c fiber.Ctx, start time.Time) error {
	if wait := loginFailureFloor - time.Since(start); wait > 0 {
		time.Sleep(wait)
	}

	return response.Unauthorized(c, badCredentials)
}

// locked reports whether one of the account's locks ("login" or "totp")
// is held. A read failure counts as locked, the safe side.
func (h Handler) locked(ctx context.Context, userID, which string) bool {
	until, err := h.Runtime.Store.User.LockedUntil(ctx, userID, which)
	if err != nil {
		h.Runtime.Log.Error("auth: lock read failed", "user", userID, "which", which, "err", err)

		return true
	}

	return until != nil && time.Now().Before(*until)
}

// recordFailure counts one wrong password or code toward its lock.
func (h Handler) recordFailure(ctx context.Context, userID, which string, limit int, lock time.Duration) {
	held, err := h.Runtime.Store.User.RecordFailure(ctx, userID, which, limit, time.Now().UTC().Add(lock))
	if err != nil {
		h.Runtime.Log.Error("auth: failure count failed", "user", userID, "which", which, "err", err)
	}

	if held {
		h.Runtime.Log.Warn("auth: account locked after repeated failures", "user", userID, "which", which, "for", lock)
	}
}

// clearFailures forgets the count after a right answer.
func (h Handler) clearFailures(ctx context.Context, userID, which string) {
	if err := h.Runtime.Store.User.ClearFailures(ctx, userID, which); err != nil {
		h.Runtime.Log.Error("auth: failure reset failed", "user", userID, "which", which, "err", err)
	}
}

// checkPassword is the password check with the lock around it: a locked
// account is refused without the password being looked at, a wrong
// password counts, a right one clears the count.
func (h Handler) checkPassword(ctx context.Context, u *usermodel.User, password string) (ok, locked bool) {
	if h.locked(ctx, u.ID, "login") {
		_ = bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))

		return false, true
	}

	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		h.recordFailure(ctx, u.ID, "login", loginMaxFailures, loginLockout)

		return false, false
	}

	h.clearFailures(ctx, u.ID, "login")

	return true, false
}

// refuseReauth confirms the password for a sensitive change and, when it
// fails, answers it. 403, not 401: the session is fine and the console
// reads 401 as an expired one. A lock says it is a lock, the right
// password would not have helped.
func (h Handler) refuseReauth(c fiber.Ctx, u *usermodel.User, password string) (error, bool) {
	ok, locked := h.checkPassword(c, u, password)
	if ok {
		return nil, false
	}

	msg := "the current password is incorrect"
	if locked {
		msg = "this account is locked for a while after failed sign-in attempts, try again later"
	}

	return response.Forbidden(c, msg), true
}

// AddressKey is how two requests count as the same address: an IPv4
// address as it is, an IPv6 one by its /64, which is what one person
// usually holds.
func AddressKey(ip string) string {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ip
	}

	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String()
	}

	p, err := addr.Prefix(64)
	if err != nil {
		return ip
	}

	return p.String()
}
