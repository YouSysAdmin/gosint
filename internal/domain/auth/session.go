// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	sessionmodel "github.com/yousysadmin/gosint/internal/models/session"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// CookieName is the session cookie.
const CookieName = "gosint_session"

// hashToken is what the sessions table stores for a cookie value.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))

	return hex.EncodeToString(sum[:])
}

// startSession stores a new session for u and sets its cookie.
func startSession(c fiber.Ctx, rt *env.Runtime, u *usermodel.User) error {
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now().UTC()

	sess := &sessionmodel.Session{
		ID:        ids.New(),
		UserID:    u.ID,
		TokenHash: hashToken(token),
		UserAgent: truncate(c.Get(fiber.HeaderUserAgent), 512),
		ClientIP:  c.IP(),
		CreatedAt: now,
		ExpiresAt: now.Add(rt.Config.Auth.SessionTTL),
	}
	if err := rt.Store.Session.Create(c, sess); err != nil {
		return err
	}

	setCookie(c, rt, token, sess.ExpiresAt)

	return nil
}

// secureCookie says whether a cookie gets the Secure flag: a request
// over TLS, or an install whose public URL is https, which covers TLS
// ended at a proxy the server was not told to trust.
func secureCookie(c fiber.Ctx, rt *env.Runtime) bool {
	return c.Scheme() == "https" || strings.HasPrefix(strings.ToLower(rt.Config.Server.PublicURL), "https://")
}

func setCookie(c fiber.Ctx, rt *env.Runtime, token string, expires time.Time) {
	c.Cookie(&fiber.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HTTPOnly: true,
		Secure:   secureCookie(c, rt),
		SameSite: fiber.CookieSameSiteLaxMode,
	})
}

// deadlineOr is next, held back to deadline when there is one.
func deadlineOr(deadline, next time.Time) time.Time {
	if deadline.IsZero() || next.Before(deadline) {
		return next
	}

	return deadline
}

// clearCookie removes the session cookie from the browser.
func clearCookie(c fiber.Ctx, rt *env.Runtime) {
	setCookie(c, rt, "", time.Unix(0, 0))
}

// Resolve returns the active account behind a session cookie value, or
// nil. A session past half its lifetime is renewed, and renewed says so,
// so the cookie is re-sent only then. Renewal never carries a session
// past auth.session_max_life from its sign-in, after which it is gone.
func Resolve(ctx context.Context, rt *env.Runtime, token string) (u *usermodel.User, sess *sessionmodel.Session, renewed bool, err error) {
	if token == "" {
		return nil, nil, false, nil
	}

	now := time.Now().UTC()
	sess, err = rt.Store.Session.Lookup(ctx, hashToken(token), now)
	if err != nil || sess == nil {
		return nil, nil, false, err
	}

	var deadline time.Time
	if maxLife := rt.Config.Auth.SessionMaxLife; maxLife > 0 {
		deadline = sess.CreatedAt.Add(maxLife)
		if !now.Before(deadline) {
			return nil, nil, false, nil
		}
	}

	u, err = rt.Store.User.Get(ctx, sess.UserID)
	if err != nil || u == nil || !u.Active {
		return nil, nil, false, err
	}

	if ttl := rt.Config.Auth.SessionTTL; sess.ExpiresAt.Sub(now) < ttl/2 && sess.ExpiresAt.Before(deadlineOr(deadline, now.Add(ttl))) {
		sess.ExpiresAt = deadlineOr(deadline, now.Add(ttl))
		if err := rt.Store.Session.Extend(ctx, sess.ID, sess.ExpiresAt); err != nil {
			return nil, nil, false, err
		}

		renewed = true
	}

	return u, sess, renewed, nil
}

// RenewCookie re-sends the cookie with the expiry Resolve moved.
func RenewCookie(c fiber.Ctx, rt *env.Runtime, token string, sess *sessionmodel.Session) {
	setCookie(c, rt, token, sess.ExpiresAt)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n]
}

// ListSessions answers the caller's live sign-ins, the current one
// marked.
func (h Handler) ListSessions(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Session.ListForUser(c, access.User(c).ID, time.Now().UTC())
	if err != nil {
		return response.Internal(c, err)
	}

	current := access.Session(c).ID
	out := sessionsResponse{Sessions: make([]sessionView, 0, len(list))}
	for _, s := range list {
		out.Sessions = append(out.Sessions, sessionView{ID: s.ID, UserAgent: s.UserAgent, ClientIP: s.ClientIP,
			CreatedAt: s.CreatedAt, ExpiresAt: s.ExpiresAt, Current: s.ID == current})
	}

	return response.Success(c, out)
}

// RevokeSession ends one of the caller's sessions. The current one is
// allowed and behaves like signing out.
func (h Handler) RevokeSession(c fiber.Ctx) error {
	id := c.Params("id")
	gone, err := h.Runtime.Store.Session.DeleteByID(c, access.User(c).ID, id)
	if err != nil {
		return response.Internal(c, err)
	}

	if !gone {
		return response.NotFound(c, "session not found")
	}

	if id == access.Session(c).ID {
		clearCookie(c, h.Runtime)
	}

	return response.Success(c, revokedResponse{Revoked: 1})
}

// RevokeOtherSessions signs the caller out everywhere but here.
func (h Handler) RevokeOtherSessions(c fiber.Ctx) error {
	n, err := h.Runtime.Store.Session.DeleteOthers(c, access.User(c).ID, access.Session(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, revokedResponse{Revoked: n})
}

// HasLiveSession reports whether token is a session that has not ended,
// for a decision taken before the request is routed.
func HasLiveSession(ctx context.Context, rt *env.Runtime, token string) bool {
	if token == "" {
		return false
	}

	sess, err := rt.Store.Session.Lookup(ctx, hashToken(token), time.Now().UTC())

	return err == nil && sess != nil
}
