// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/mailer"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/user"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
)

// An invitation admits a person who has no account yet: the link lets
// them register while registration is closed, or sign in, and lands
// them on the investigation with the chosen role. The clear token
// leaves once, in the link (mailed when mail is configured, shown to
// the inviter either way), only its hash is stored.

// invitationLink is where a token is redeemed.
func invitationLink(rt *env.Runtime, token string) string {
	return strings.TrimRight(rt.Config.Server.PublicURL, "/") + env.ConsolePath + "/invitations/" + token
}

// ListInvitations answers the investigation's open invitations.
func (h Handler) ListInvitations(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Investigation.PendingInvitations(c, access.Investigation(c).Investigation.ID, time.Now().UTC())
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, invitationsResponse{Invitations: list})
}

// CreateInvitation invites an address. An address that already holds
// an account and is a member needs no invitation.
func (h Handler) CreateInvitation(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[addMemberInput](c)
	if !ok {
		return resp
	}

	scope, inviter := access.Investigation(c), access.User(c)
	if u, err := h.Runtime.Store.User.GetByEmail(c, in.Email); err != nil {
		return response.Internal(c, err)
	} else if u != nil {
		role, err := h.Runtime.Store.Investigation.Role(c, scope.Investigation.ID, u.ID)
		if err != nil {
			return response.Internal(c, err)
		}

		if role != "" {
			return response.Conflict(c, "already a member")
		}
	}

	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := hex.EncodeToString(raw)
	now := time.Now().UTC()
	inv := &invmodel.Invitation{ID: ids.New(), InvestigationID: scope.Investigation.ID, Email: in.Email, Role: in.Role,
		TokenHash: invmodel.HashInvitationToken(token), InvitedBy: inviter.ID, CreatedAt: now, ExpiresAt: now.Add(invmodel.InvitationTTL)}
	if err := h.Runtime.Store.Investigation.CreateInvitation(c, inv); err != nil {
		return response.Internal(c, err)
	}

	link := invitationLink(h.Runtime, token)
	emailed := h.Runtime.Mail.Enabled()
	if emailed {
		subject, htmlBody, textBody := mailer.Invitation(scope.Investigation.Name, inviter.Email, link, int(invmodel.InvitationTTL.Hours()))
		mailer.SendAsync(h.Runtime.Mail, h.Runtime.Log, []string{in.Email}, subject, htmlBody, textBody)
	}

	return response.Created(c, invitationCreatedResponse{Invitation: *inv, Link: link, Emailed: emailed})
}

// DeleteInvitation revokes one.
func (h Handler) DeleteInvitation(c fiber.Ctx) error {
	gone, err := h.Runtime.Store.Investigation.DeleteInvitation(c, access.Investigation(c).Investigation.ID, c.Params("inv_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !gone {
		return response.NotFound(c, "invitation not found")
	}

	return response.NoContent(c)
}

// LookupInvitation tells the person opening a link what it is for and
// how they can take it: whether the address has an account, and who is
// signed in right now. Public, the token is the claim.
func (h Handler) LookupInvitation(c fiber.Ctx) error {
	inv, resp, ok := h.openInvitation(c)
	if !ok {
		return resp
	}

	investigation, err := h.Runtime.Store.Investigation.Get(c, inv.InvestigationID)
	if err != nil {
		return response.Internal(c, err)
	}

	if investigation == nil {
		return response.NotFound(c, "invitation not found")
	}

	account, err := h.Runtime.Store.User.GetByEmail(c, inv.Email)
	if err != nil {
		return response.Internal(c, err)
	}

	out := invitationLookupResponse{Investigation: investigation.Name, Email: inv.Email, Role: inv.Role,
		ExpiresAt: inv.ExpiresAt, HasAccount: account != nil && account.Active}
	if u := access.MaybeUser(c); u != nil {
		out.SignedInAs = u.Email
	}

	return response.Success(c, out)
}

// AcceptInvitation makes the signed-in account a member. The account's
// email must be the invited one, so a leaked link alone admits nobody.
func (h Handler) AcceptInvitation(c fiber.Ctx) error {
	inv, resp, ok := h.openInvitation(c)
	if !ok {
		return resp
	}

	u := access.User(c)
	if !strings.EqualFold(inv.Email, u.Email) {
		return response.Forbidden(c, "this invitation was issued to a different email address")
	}

	now := time.Now().UTC()
	m := &invmodel.Member{InvestigationID: inv.InvestigationID, UserID: u.ID, Role: inv.Role, CreatedAt: now}
	err := h.Runtime.Store.Investigation.AddMember(c, m)
	if err != nil && !database.UniqueViolation(err) {
		return response.Internal(c, err)
	}

	if _, err := h.Runtime.Store.Investigation.AcceptInvitation(c, inv.ID, now); err != nil {
		return response.Internal(c, err)
	}

	role, err := h.Runtime.Store.Investigation.Role(c, inv.InvestigationID, u.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, invitationAcceptedResponse{InvestigationID: inv.InvestigationID,
		Member: Member{UserID: u.ID, Role: role, User: user.ProfileOf(u)}})
}

// openInvitation reads the token in the path: 404 for an unknown or
// taken one, 410 for one that expired.
func (h Handler) openInvitation(c fiber.Ctx) (*invmodel.Invitation, error, bool) {
	inv, err := h.Runtime.Store.Investigation.InvitationByHash(c, invmodel.HashInvitationToken(c.Params("token")))
	if err != nil {
		return nil, response.Internal(c, err), false
	}

	if inv == nil || inv.AcceptedAt != nil {
		return nil, response.NotFound(c, "invitation not found"), false
	}

	if !time.Now().UTC().Before(inv.ExpiresAt) {
		return nil, response.Gone(c, "this invitation has expired, ask for a new one"), false
	}

	return inv, nil, true
}

// TransferOwner hands the investigation to another member. Owner only.
func (h Handler) TransferOwner(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[transferInput](c)
	if !ok {
		return resp
	}

	scope := access.Investigation(c)
	if scope.Role != invmodel.RoleOwner {
		return response.Forbidden(c, "only the owner can hand the investigation over")
	}

	if in.UserID == access.User(c).ID {
		return response.BadRequest(c, "you own it already")
	}

	moved, err := h.Runtime.Store.Investigation.TransferOwner(c, scope.Investigation.ID, access.User(c).ID, in.UserID, time.Now().UTC())
	if err != nil {
		return response.Internal(c, err)
	}

	if !moved {
		return response.NotFound(c, "no such member")
	}

	return response.NoContent(c)
}
