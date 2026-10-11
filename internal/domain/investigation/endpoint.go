// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/ids"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	"github.com/yousysadmin/gosint/internal/database"
	"github.com/yousysadmin/gosint/internal/domain/attachment"
	"github.com/yousysadmin/gosint/internal/domain/user"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// Handler serves the investigation and membership endpoints. Every
// route with an :id is behind requireInvestigation, which has already
// checked the caller's role.
type Handler struct {
	Runtime *env.Runtime
}

// List answers the caller's investigations.
func (h Handler) List(c fiber.Ctx) error {
	list, err := h.Runtime.Store.Investigation.ListForUser(c, access.User(c).ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, investigationsResponse{Investigations: list})
}

// Create starts an investigation owned by the caller.
func (h Handler) Create(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[createInput](c)
	if !ok {
		return resp
	}

	now := time.Now().UTC()
	inv := &invmodel.Investigation{
		ID:          ids.New(),
		Name:        in.Name,
		Description: in.Description,
		Status:      "active",
		OwnerID:     access.User(c).ID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.Runtime.Store.Investigation.Create(c, inv); err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, investigationResponse{
		Investigation: invmodel.WithRole{Investigation: *inv, Role: invmodel.RoleOwner},
	})
}

// Get answers one investigation with its sketches.
func (h Handler) Get(c fiber.Ctx) error {
	scope := access.Investigation(c)
	sketches, err := h.Runtime.Store.Sketch.ListByInvestigation(c, scope.Investigation.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, investigationResponse{
		Investigation: invmodel.WithRole{Investigation: *scope.Investigation, Role: scope.Role},
		Sketches:      sketches,
	})
}

// Update changes name, description or status.
func (h Handler) Update(c fiber.Ctx) error {
	scope := access.Investigation(c)
	inv := *scope.Investigation
	in, resp, ok := validation.BindOnto(c, updateInput{Name: inv.Name, Description: inv.Description, Status: inv.Status})
	if !ok {
		return resp
	}

	inv.Name, inv.Description, inv.Status = in.Name, in.Description, in.Status
	inv.UpdatedAt = time.Now().UTC()
	if err := h.Runtime.Store.Investigation.Update(c, &inv); err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, investigationResponse{Investigation: invmodel.WithRole{Investigation: inv, Role: scope.Role}})
}

// Delete removes the investigation with every sketch and graph in it.
func (h Handler) Delete(c fiber.Ctx) error {
	id := access.Investigation(c).Investigation.ID
	sketches, err := h.Runtime.Store.Sketch.ListByInvestigation(c, id)
	if err != nil {
		return response.Internal(c, err)
	}

	// The sketches cascade with their file records, the bytes follow.
	var hashes []string
	for _, sk := range sketches {
		more, err := h.Runtime.Store.Attachment.HashesOfSketch(c, sk.ID)
		if err != nil {
			return response.Internal(c, err)
		}

		hashes = append(hashes, more...)
	}

	if err := h.Runtime.Store.Investigation.Delete(c, id); err != nil {
		return response.Internal(c, err)
	}

	attachment.Release(c, h.Runtime, hashes)

	return response.NoContent(c)
}

// Members answers who works on the investigation.
func (h Handler) Members(c fiber.Ctx) error {
	members, err := h.Runtime.Store.Investigation.Members(c, access.Investigation(c).Investigation.ID)
	if err != nil {
		return response.Internal(c, err)
	}

	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.UserID)
	}

	users, err := h.Runtime.Store.User.GetMany(c, ids)
	if err != nil {
		return response.Internal(c, err)
	}

	byID := make(map[string]*usermodel.User, len(users))
	for i := range users {
		byID[users[i].ID] = &users[i]
	}

	out := make([]Member, 0, len(members))
	for _, m := range members {
		if u := byID[m.UserID]; u != nil {
			out = append(out, Member{UserID: m.UserID, Role: m.Role, User: user.ProfileOf(u)})
		}
	}

	return response.Success(c, membersResponse{Members: out})
}

// AddMember shares the investigation with an existing account.
func (h Handler) AddMember(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[addMemberInput](c)
	if !ok {
		return resp
	}

	u, err := h.Runtime.Store.User.GetByEmail(c, in.Email)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil || !u.Active {
		return response.NotFound(c, "no account with that email")
	}

	m := &invmodel.Member{
		InvestigationID: access.Investigation(c).Investigation.ID,
		UserID:          u.ID,
		Role:            in.Role,
		CreatedAt:       time.Now().UTC(),
	}

	err = h.Runtime.Store.Investigation.AddMember(c, m)
	if database.UniqueViolation(err) {
		return response.Conflict(c, "already a member")
	}

	if err != nil {
		return response.Internal(c, err)
	}

	return response.Created(c, memberResponse{Member: Member{UserID: u.ID, Role: m.Role, User: user.ProfileOf(u)}})
}

// SetMemberRole changes a member's role. The owner's cannot change.
func (h Handler) SetMemberRole(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[setRoleInput](c)
	if !ok {
		return resp
	}

	invID, userID := access.Investigation(c).Investigation.ID, c.Params("user_id")
	found, err := h.Runtime.Store.Investigation.SetMemberRole(c, invID, userID, in.Role)
	if err != nil {
		return response.Internal(c, err)
	}

	if !found {
		return response.NotFound(c, "no such member, or the owner")
	}

	u, err := h.Runtime.Store.User.Get(c, userID)
	if err != nil {
		return response.Internal(c, err)
	}

	if u == nil {
		return response.NotFound(c, "no such account")
	}

	return response.Success(c, memberResponse{Member: Member{UserID: userID, Role: in.Role, User: user.ProfileOf(u)}})
}

// RemoveMember takes a member off the investigation. The owner stays.
func (h Handler) RemoveMember(c fiber.Ctx) error {
	found, err := h.Runtime.Store.Investigation.RemoveMember(c, access.Investigation(c).Investigation.ID, c.Params("user_id"))
	if err != nil {
		return response.Internal(c, err)
	}

	if !found {
		return response.NotFound(c, "no such member, or the owner")
	}

	return response.NoContent(c)
}
