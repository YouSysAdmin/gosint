// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/yousysadmin/gosint/internal/database"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
)

const invitationSelect = `
SELECT id, investigation_id, email, role, token_hash, invited_by, created_at, expires_at, accepted_at
FROM investigation_invitations`

// CreateInvitation inserts inv.
func (s *Store) CreateInvitation(ctx context.Context, inv *invmodel.Invitation) error {
	email, err := s.codec.SealText(emailAAD(inv.ID), inv.Email)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		INSERT INTO investigation_invitations (id, investigation_id, email, role, token_hash, invited_by, created_at, expires_at, accepted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.InvestigationID, email, inv.Role, inv.TokenHash, inv.InvitedBy, inv.CreatedAt, inv.ExpiresAt,
		database.NullTime(inv.AcceptedAt))

	return err
}

// InvitationByHash is the lookup a link makes, not scoped by anything:
// the token is the claim.
func (s *Store) InvitationByHash(ctx context.Context, hash string) (*invmodel.Invitation, error) {
	inv, err := s.scanInvitation(s.QueryRow(ctx, invitationSelect+` WHERE token_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return inv, err
}

// PendingInvitations lists what the investigation still has open.
func (s *Store) PendingInvitations(ctx context.Context, investigationID string, now time.Time) ([]invmodel.Invitation, error) {
	rows, err := s.Query(ctx, invitationSelect+`
		WHERE investigation_id = ? AND accepted_at IS NULL AND expires_at > ?
		ORDER BY created_at`, investigationID, now)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []invmodel.Invitation{}
	for rows.Next() {
		inv, err := s.scanInvitation(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *inv)
	}

	return out, rows.Err()
}

// PendingInvitationFor is the open invitation of an address to any
// investigation, for a registration while the door is closed.
// The address is sealed, so it is compared once the row is read.
func (s *Store) PendingInvitationFor(ctx context.Context, hash, email string, now time.Time) (*invmodel.Invitation, error) {
	inv, err := s.scanInvitation(s.QueryRow(ctx, invitationSelect+`
		WHERE token_hash = ? AND accepted_at IS NULL AND expires_at > ?`, hash, now))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && inv.Email != email) {
		return nil, nil
	}

	return inv, err
}

// DeleteInvitation revokes one, false when the investigation had none
// by that id.
func (s *Store) DeleteInvitation(ctx context.Context, investigationID, id string) (bool, error) {
	res, err := s.Exec(ctx, `DELETE FROM investigation_invitations WHERE investigation_id = ? AND id = ?`, investigationID, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// AcceptInvitation marks one accepted, once.
func (s *Store) AcceptInvitation(ctx context.Context, id string, at time.Time) (bool, error) {
	res, err := s.Exec(ctx, `UPDATE investigation_invitations SET accepted_at = ? WHERE id = ? AND accepted_at IS NULL`, at, id)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// OwnedBy lists the investigations userID owns.
func (s *Store) OwnedBy(ctx context.Context, userID string) ([]invmodel.Investigation, error) {
	rows, err := s.Query(ctx, `SELECT `+investigationColumns+` FROM investigations i WHERE i.owner_id = ?`, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	out := []invmodel.Investigation{}
	for rows.Next() {
		inv, err := s.scanInvestigation(rows)
		if err != nil {
			return nil, err
		}

		out = append(out, *inv)
	}

	// By name, sorted here since the names are sealed.
	slices.SortFunc(out, func(a, b invmodel.Investigation) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})

	return out, rows.Err()
}

// TransferOwner hands the investigation to another member in one
// transaction: the old owner becomes an admin, the new one the owner.
// False when toUserID is not a member.
func (s *Store) TransferOwner(ctx context.Context, investigationID, fromUserID, toUserID string, now time.Time) (bool, error) {
	moved := false
	err := s.Tx(ctx, func(tx *database.Tx) error {
		res, err := tx.Exec(ctx, `UPDATE investigation_members SET role = ? WHERE investigation_id = ? AND user_id = ?`,
			invmodel.RoleOwner, investigationID, toUserID)
		if err != nil {
			return err
		}

		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}

		if _, err := tx.Exec(ctx, `UPDATE investigation_members SET role = ? WHERE investigation_id = ? AND user_id = ?`,
			invmodel.RoleAdmin, investigationID, fromUserID); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `UPDATE investigations SET owner_id = ?, updated_at = ? WHERE id = ?`, toUserID, now, investigationID)
		moved = err == nil

		return err
	})

	return moved, err
}

func emailAAD(id string) string { return database.AAD("investigation_invitations", "email", id) }

func (s *Store) scanInvitation(r interface{ Scan(...any) error }) (*invmodel.Invitation, error) {
	var inv invmodel.Invitation
	var accepted sql.NullTime
	if err := r.Scan(&inv.ID, &inv.InvestigationID, &inv.Email, &inv.Role, &inv.TokenHash, &inv.InvitedBy,
		&inv.CreatedAt, &inv.ExpiresAt, &accepted); err != nil {
		return nil, err
	}

	email, err := s.codec.OpenText(emailAAD(inv.ID), inv.Email)
	if err != nil {
		return nil, err
	}

	inv.Email = email

	if accepted.Valid {
		inv.AcceptedAt = &accepted.Time
	}

	return &inv, nil
}
