// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/ids"
	corepasskey "github.com/yousysadmin/gosint/internal/core/passkey"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	pkmodel "github.com/yousysadmin/gosint/internal/models/passkey"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// Passkeys, ported from mailyard. A ceremony is two requests: begin
// hands the browser its options and seals the library's session into a
// cookie, finish takes the cookie back, spends its challenge in the
// database (so a copy answers nowhere, on any node) and verifies.
const (
	// Separate cookies for the two ceremonies: one name would let a
	// half-finished enrolment be handed to the login verifier.
	passkeyRegCookie   = "gosint_passkey_reg"   //nolint:gosec // a cookie name, not a credential
	passkeyLoginCookie = "gosint_passkey_login" //nolint:gosec // a cookie name, not a credential
	passkeyCeremonyTTL = 5 * time.Minute
	maxPasskeyName     = 60
	maxPasskeysPerUser = 20
)

// passkeysAvailable says the feature is on.
func (h Handler) passkeysAvailable() bool { return h.Runtime.Config.Auth.PasskeysEnabled }

// webauthnService builds the relying party from the browser's Origin,
// which is what the browser signs into the client data. Taken on its
// own word the header would make the RP whatever the caller declared,
// so its host must be the public URL's host or under it when one is
// configured.
func (h Handler) webauthnService(c fiber.Ctx) (*corepasskey.Service, error) {
	origin := c.Get(fiber.HeaderOrigin)
	if origin == "" {
		return nil, errors.New("passkeys need a browser that sends an Origin header")
	}

	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return nil, errors.New("the Origin header is not a usable origin")
	}

	if public, err := url.Parse(h.Runtime.Config.Server.PublicURL); err == nil && public.Hostname() != "" &&
		!sameOrUnder(u.Hostname(), public.Hostname()) {
		return nil, fmt.Errorf("the origin %q is not the configured server.public_url host", u.Hostname())
	}

	return corepasskey.New(u.Hostname(), origin)
}

// sameOrUnder reports whether host is name or a subdomain of it.
func sameOrUnder(host, name string) bool {
	host, name = strings.ToLower(host), strings.ToLower(name)

	return host == name || strings.HasSuffix(host, "."+name)
}

// takeCeremony reads the ceremony cookie, clears it and spends its
// challenge, so a challenge is answerable exactly once.
func (h Handler) takeCeremony(c fiber.Ctx, name string) (*corepasskey.SessionData, error) {
	sd, err := openCookie[corepasskey.SessionData](c, h.Runtime, name)
	clearNamed(c, h.Runtime, name)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	expires := sd.Expires
	if expires.IsZero() {
		expires = now.Add(passkeyCeremonyTTL)
	}

	first, err := h.Runtime.Store.Passkey.SpendChallenge(c, sd.Challenge, expires, now)
	if err != nil {
		return nil, err
	}

	if !first {
		return nil, errors.New("ceremony already used")
	}

	return sd, nil
}

// passkeySelf is the signed-in local account, refused when passkeys
// are off.
func (h Handler) passkeySelf(c fiber.Ctx) (*usermodel.User, error, bool) {
	if !h.passkeysAvailable() {
		return nil, response.Forbidden(c, "passkeys are not enabled on this install"), false
	}

	return h.localSelf(c)
}

// webauthnUser is the library's view of an account: the handle plus
// every credential enrolled. The handle is the account id.
func (h Handler) webauthnUser(c fiber.Ctx, u *usermodel.User) (*corepasskey.User, error) {
	rows, err := h.Runtime.Store.Passkey.ListForUser(c, u.ID)
	if err != nil {
		return nil, err
	}

	creds := make([]corepasskey.Credential, 0, len(rows))
	for _, r := range rows {
		cred, err := corepasskey.DecodeCredential(r.Credential)
		if err != nil {
			// One unreadable row must not make the account unusable.
			h.Runtime.Log.Error("auth: stored passkey is unreadable", "passkey", r.ID, "err", err)

			continue
		}

		creds = append(creds, *cred)
	}

	return &corepasskey.User{Handle: []byte(u.ID), Name: u.Email, Display: u.Email, Creds: creds}, nil
}

// PasskeyList answers the caller's passkeys.
func (h Handler) PasskeyList(c fiber.Ctx) error {
	u, resp, ok := h.passkeySelf(c)
	if !ok {
		return resp
	}

	rows, err := h.Runtime.Store.Passkey.ListForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, passkeyListResponse{Passkeys: rows})
}

// PasskeyRegisterBegin starts enrolment behind the password.
func (h Handler) PasskeyRegisterBegin(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[reauthInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.passkeySelf(c)
	if !ok {
		return resp
	}

	if refusal, refused := h.refuseReauth(c, u, in.Password); refused {
		return refusal
	}

	n, err := h.Runtime.Store.Passkey.CountForUser(c, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	if n >= maxPasskeysPerUser {
		return response.Conflict(c, fmt.Sprintf("this account already holds %d passkeys, remove one first", n))
	}

	svc, err := h.webauthnService(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	wu, err := h.webauthnUser(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	opts, sess, err := svc.BeginRegistration(wu)
	if err != nil {
		return response.Internal(c, err)
	}

	if err := sealCookie(c, h.Runtime, passkeyRegCookie, sess, passkeyCeremonyTTL); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, opts)
}

// PasskeyRegisterFinish verifies the attestation and stores the
// credential. The body is the raw WebAuthn attestation as the browser
// produced it, so the label rides in ?name=.
func (h Handler) PasskeyRegisterFinish(c fiber.Ctx) error {
	u, resp, ok := h.passkeySelf(c)
	if !ok {
		return resp
	}

	svc, err := h.webauthnService(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	sess, err := h.takeCeremony(c, passkeyRegCookie)
	if err != nil {
		return response.BadRequest(c, "the enrolment expired, start again")
	}

	wu, err := h.webauthnUser(c, u)
	if err != nil {
		return response.Internal(c, err)
	}

	cred, err := svc.FinishRegistration(wu, *sess, c.Body())
	if err != nil {
		h.Runtime.Log.Warn("auth: passkey enrolment rejected", "user", u.ID, "err", err)

		return response.BadRequest(c, "the passkey could not be verified")
	}

	encoded, err := corepasskey.EncodeCredential(cred)
	if err != nil {
		return response.Internal(c, err)
	}

	row := &pkmodel.Passkey{ID: ids.New(), UserID: u.ID, CredentialID: base64.RawURLEncoding.EncodeToString(cred.ID),
		Name: passkeyLabel(c.Query("name")), Credential: encoded, CreatedAt: time.Now().UTC()}
	if err := h.Runtime.Store.Passkey.Put(c, row); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, passkeyResponse{Passkey: *row})
}

// PasskeyRename relabels one. No password, a label is not a way in.
func (h Handler) PasskeyRename(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[passkeyRenameInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.passkeySelf(c)
	if !ok {
		return resp
	}

	renamed, err := h.Runtime.Store.Passkey.Rename(c, u.ID, c.Params("id"), passkeyLabel(in.Name))
	if err != nil {
		return response.Internal(c, err)
	}

	if !renamed {
		return response.NotFound(c, "passkey not found")
	}

	return response.NoContent(c)
}

// PasskeyDelete removes one behind the password.
func (h Handler) PasskeyDelete(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[reauthInput](c)
	if !ok {
		return resp
	}

	u, resp, ok := h.passkeySelf(c)
	if !ok {
		return resp
	}

	if refusal, refused := h.refuseReauth(c, u, in.Password); refused {
		return refusal
	}

	removed, err := h.Runtime.Store.Passkey.Delete(c, u.ID, c.Params("id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !removed {
		return response.NotFound(c, "passkey not found")
	}

	return response.NoContent(c)
}

// PasskeyLoginBegin starts a usernameless assertion. Open: nobody has
// identified themselves yet, which is the point.
func (h Handler) PasskeyLoginBegin(c fiber.Ctx) error {
	if !h.passkeysAvailable() {
		return response.BadRequest(c, "passkey sign-in is not available")
	}

	svc, err := h.webauthnService(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	opts, sess, err := svc.BeginLogin()
	if err != nil {
		return response.Internal(c, err)
	}

	if err := sealCookie(c, h.Runtime, passkeyLoginCookie, sess, passkeyCeremonyTTL); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, opts)
}

// PasskeyLoginFinish verifies the assertion and signs the owner in. A
// passkey completes sign-in on its own: user verification was required
// at both ends, possession and an unlock in one gesture, so no code is
// asked after it.
func (h Handler) PasskeyLoginFinish(c fiber.Ctx) error {
	if !h.passkeysAvailable() {
		return response.BadRequest(c, "passkey sign-in is not available")
	}

	svc, err := h.webauthnService(c)
	if err != nil {
		return response.BadRequest(c, err.Error())
	}

	sess, err := h.takeCeremony(c, passkeyLoginCookie)
	if err != nil {
		return response.BadRequest(c, "the sign-in expired, start again")
	}

	var (
		matched *usermodel.User
		row     *pkmodel.Passkey
	)

	// The credential id IS the identity claim, the signature checked
	// afterwards makes it true. The handle must agree.
	resolve := func(rawID, userHandle []byte) (*corepasskey.User, error) {
		pk, err := h.Runtime.Store.Passkey.GetByCredential(c, base64.RawURLEncoding.EncodeToString(rawID))
		if err != nil {
			return nil, err
		}

		if pk == nil || pk.UserID != string(userHandle) {
			return nil, errors.New("unknown passkey")
		}

		u, err := h.Runtime.Store.User.Get(c, pk.UserID)
		if err != nil {
			return nil, err
		}

		if u == nil || !u.Active {
			return nil, errors.New("account unavailable")
		}

		cred, err := corepasskey.DecodeCredential(pk.Credential)
		if err != nil {
			return nil, err
		}

		matched, row = u, pk

		return &corepasskey.User{Handle: []byte(u.ID), Name: u.Email, Display: u.Email, Creds: []corepasskey.Credential{*cred}}, nil
	}

	cred, _, err := svc.FinishLogin(resolve, *sess, c.Body())
	if err != nil || matched == nil || row == nil {
		h.Runtime.Log.Warn("auth: passkey sign-in failed", "err", err)

		return response.Unauthorized(c, "passkey sign-in failed")
	}

	// A sign counter that did not advance means the credential may
	// have been copied. Synced passkeys report zero and never trip this.
	if cred.Authenticator.CloneWarning {
		h.Runtime.Log.Warn("auth: passkey clone warning, sign-in refused", "user", matched.ID)

		return response.Unauthorized(c, "passkey sign-in failed")
	}

	if err := startSession(c, h.Runtime, matched); err != nil {
		return response.Internal(c, err)
	}

	if encoded, eerr := corepasskey.EncodeCredential(cred); eerr == nil {
		if uerr := h.Runtime.Store.Passkey.RecordUse(c, row.ID, encoded, time.Now().UTC()); uerr != nil {
			h.Runtime.Log.Warn("auth: recording passkey use failed", "passkey", row.ID, "err", uerr)
		}
	}

	p, err := h.profileOf(c, matched)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, loginResponse{User: &p})
}

// passkeyLabel cleans the label: valid UTF-8, trimmed, cut at
// maxPasskeyName characters, a default when nothing is left.
func passkeyLabel(raw string) string {
	name := []rune(strings.TrimSpace(strings.ToValidUTF8(raw, "")))
	if len(name) > maxPasskeyName {
		name = name[:maxPasskeyName]
	}

	if s := strings.TrimSpace(string(name)); s != "" {
		return s
	}

	return "Passkey"
}
