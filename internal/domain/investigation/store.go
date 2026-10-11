// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import (
	"context"
	"database/sql"
	"errors"

	"github.com/yousysadmin/gosint/internal/database"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
)

// Store persists investigations and their members, the name and the
// description sealed at rest.
type Store struct {
	database.Base
	codec database.Codec
}

// NewStore binds the investigation store to db.
func NewStore(db database.Database, codec database.Codec) *Store {
	return &Store{Base: database.NewBase(db), codec: codec}
}

const investigationColumns = `i.id, i.name, i.description, i.status, i.owner_id, i.created_at, i.updated_at`

func aad(column, id string) string { return database.AAD("investigations", column, id) }

func (s *Store) scanInvestigation(row interface{ Scan(...any) error }, extra ...any) (*invmodel.Investigation, error) {
	var inv invmodel.Investigation
	dest := append([]any{&inv.ID, &inv.Name, &inv.Description, &inv.Status, database.Str(&inv.OwnerID),
		&inv.CreatedAt, &inv.UpdatedAt}, extra...)
	if err := row.Scan(dest...); err != nil {
		return nil, err
	}

	name, err1 := s.codec.OpenText(aad("name", inv.ID), inv.Name)
	desc, err2 := s.codec.OpenText(aad("description", inv.ID), inv.Description)
	if err := errors.Join(err1, err2); err != nil {
		return nil, err
	}

	inv.Name, inv.Description = name, desc

	return &inv, nil
}

// sealed is inv's name and description as stored.
func (s *Store) sealed(inv *invmodel.Investigation) (string, string, error) {
	name, err1 := s.codec.SealText(aad("name", inv.ID), inv.Name)
	desc, err2 := s.codec.SealText(aad("description", inv.ID), inv.Description)

	return name, desc, errors.Join(err1, err2)
}

// Create inserts the investigation and its owner as the first member.
func (s *Store) Create(ctx context.Context, inv *invmodel.Investigation) error {
	name, desc, err := s.sealed(inv)
	if err != nil {
		return err
	}

	return s.Tx(ctx, func(tx *database.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO investigations (id, name, description, status, owner_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			inv.ID, name, desc, inv.Status, database.NullStr(inv.OwnerID), inv.CreatedAt, inv.UpdatedAt)
		if err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO investigation_members (investigation_id, user_id, role, created_at)
			VALUES (?, ?, ?, ?)`,
			inv.ID, inv.OwnerID, invmodel.RoleOwner, inv.CreatedAt)

		return err
	})
}

// Get returns one investigation.
func (s *Store) Get(ctx context.Context, id string) (*invmodel.Investigation, error) {
	inv, err := s.scanInvestigation(s.QueryRow(ctx, `SELECT `+investigationColumns+` FROM investigations i WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return inv, err
}

// ListForUser lists userID's investigations with their role, most
// recently changed first.
func (s *Store) ListForUser(ctx context.Context, userID string) ([]invmodel.WithRole, error) {
	rows, err := s.Query(ctx, `
		SELECT `+investigationColumns+`, m.role
		FROM investigations i
		JOIN investigation_members m ON m.investigation_id = i.id
		WHERE m.user_id = ?
		ORDER BY i.updated_at DESC, i.id`, userID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []invmodel.WithRole
	for rows.Next() {
		var role invmodel.Role
		inv, err := s.scanInvestigation(rows, &role)
		if err != nil {
			return nil, err
		}

		out = append(out, invmodel.WithRole{Investigation: *inv, Role: role})
	}

	return out, rows.Err()
}

// Update writes the editable fields.
func (s *Store) Update(ctx context.Context, inv *invmodel.Investigation) error {
	name, desc, err := s.sealed(inv)
	if err != nil {
		return err
	}

	_, err = s.Exec(ctx, `
		UPDATE investigations SET name = ?, description = ?, status = ?, updated_at = ?
		WHERE id = ?`,
		name, desc, inv.Status, inv.UpdatedAt, inv.ID)

	return err
}

// Delete removes the investigation. Its sketches, graphs and members go
// with it by cascade.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.Exec(ctx, `DELETE FROM investigations WHERE id = ?`, id)

	return err
}

// Role is userID's role on the investigation, "" for a non-member.
func (s *Store) Role(ctx context.Context, investigationID, userID string) (invmodel.Role, error) {
	var role invmodel.Role
	err := s.QueryRow(ctx, `
		SELECT role FROM investigation_members WHERE investigation_id = ? AND user_id = ?`,
		investigationID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	return role, err
}

// Members lists the investigation's members, owner first.
func (s *Store) Members(ctx context.Context, investigationID string) ([]invmodel.Member, error) {
	rows, err := s.Query(ctx, `
		SELECT investigation_id, user_id, role, created_at
		FROM investigation_members WHERE investigation_id = ?
		ORDER BY CASE role WHEN 'owner' THEN 0 ELSE 1 END, created_at, user_id`, investigationID)
	if err != nil {
		return nil, err
	}

	defer rows.Close() //nolint:errcheck // read-only

	var out []invmodel.Member
	for rows.Next() {
		var m invmodel.Member
		if err := rows.Scan(&m.InvestigationID, &m.UserID, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}

		out = append(out, m)
	}

	return out, rows.Err()
}

// AddMember inserts m. A second row for the same person is a unique
// violation, which the caller answers as a conflict.
func (s *Store) AddMember(ctx context.Context, m *invmodel.Member) error {
	_, err := s.Exec(ctx, `
		INSERT INTO investigation_members (investigation_id, user_id, role, created_at)
		VALUES (?, ?, ?, ?)`,
		m.InvestigationID, m.UserID, m.Role, m.CreatedAt)

	return err
}

// SetMemberRole changes a non-owner member's role.
func (s *Store) SetMemberRole(ctx context.Context, investigationID, userID string, role invmodel.Role) (bool, error) {
	res, err := s.Exec(ctx, `
		UPDATE investigation_members SET role = ?
		WHERE investigation_id = ? AND user_id = ? AND role <> ?`,
		role, investigationID, userID, invmodel.RoleOwner)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}

// RemoveMember removes a non-owner member.
func (s *Store) RemoveMember(ctx context.Context, investigationID, userID string) (bool, error) {
	res, err := s.Exec(ctx, `
		DELETE FROM investigation_members
		WHERE investigation_id = ? AND user_id = ? AND role <> ?`,
		investigationID, userID, invmodel.RoleOwner)
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()

	return n > 0, err
}
