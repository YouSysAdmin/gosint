// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"time"

	"github.com/yousysadmin/gosint/internal/domain/user"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	sketchmodel "github.com/yousysadmin/gosint/internal/models/sketch"
)

// createInput is a new investigation.
type createInput struct {
	Name        string `json:"name"        validate:"required,notblank,max=200" normalize:"trim"`
	Description string `json:"description" validate:"max=10000" normalize:"trim"`
}

// updateInput is decoded over the stored investigation, so a field left
// out keeps its value.
type updateInput struct {
	Name        string `json:"name"        validate:"required,notblank,max=200" normalize:"trim"`
	Description string `json:"description" validate:"max=10000" normalize:"trim"`
	Status      string `json:"status"      validate:"oneof=active archived"`
}

// addMemberInput shares the investigation with an existing account.
type addMemberInput struct {
	Email string        `json:"email" validate:"required,email,max=254" normalize:"normalize"`
	Role  invmodel.Role `json:"role"  validate:"required,oneof=admin editor viewer"`
}

// setRoleInput changes a member's role. Owner cannot be granted.
type setRoleInput struct {
	Role invmodel.Role `json:"role" validate:"required,oneof=admin editor viewer"`
}

// investigationResponse answers a single investigation.
type investigationResponse struct {
	Investigation invmodel.WithRole    `json:"investigation"`
	Sketches      []sketchmodel.Sketch `json:"sketches"`
}

// investigationsResponse answers the list.
type investigationsResponse struct {
	Investigations []invmodel.WithRole `json:"investigations"`
}

// Member is one person on an investigation.
type Member struct {
	UserID string        `json:"user_id"`
	Role   invmodel.Role `json:"role"`
	User   user.Profile  `json:"user"`
}

// memberResponse answers a single membership change.
type memberResponse struct {
	Member Member `json:"member"`
}

// membersResponse answers the member list.
type membersResponse struct {
	Members []Member `json:"members"`
}

// transferInput names the member who becomes the owner.
type transferInput struct {
	UserID string `json:"user_id" validate:"required,uuid"`
}

// invitationsResponse lists the open invitations.
type invitationsResponse struct {
	Invitations []invmodel.Invitation `json:"invitations"`
}

// invitationCreatedResponse answers a new invitation with its link, the
// one time the clear token is seen, and whether it was mailed too.
type invitationCreatedResponse struct {
	Invitation invmodel.Invitation `json:"invitation"`
	Link       string              `json:"link"`
	Emailed    bool                `json:"emailed"`
}

// invitationLookupResponse tells the person opening a link what it is
// for and how to take it.
type invitationLookupResponse struct {
	Investigation string        `json:"investigation"`
	Email         string        `json:"email"`
	Role          invmodel.Role `json:"role"`
	ExpiresAt     time.Time     `json:"expires_at"`

	// HasAccount says the invited address can sign in already, else it
	// registers first. SignedInAs is who holds the session now, "" for
	// nobody.
	HasAccount bool   `json:"has_account"`
	SignedInAs string `json:"signed_in_as"`
}

// invitationAcceptedResponse answers an accepted invitation.
type invitationAcceptedResponse struct {
	InvestigationID string `json:"investigation_id"`
	Member          Member `json:"member"`
}
