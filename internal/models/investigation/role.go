// Gosint, Copyright (c) 2026 YouSysAdmin

package investigation

import "slices"

// Role is what a member may do on an investigation.
type Role string

// The roles, most powerful first.
const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// Action is something a role may be allowed to do.
type Action string

// The actions a route can require.
const (
	ActionRead   Action = "read"
	ActionCreate Action = "create"
	ActionUpdate Action = "update"

	// ActionDelete removes an investigation or a sketch, owner only.
	ActionDelete Action = "delete"

	// ActionManage changes who is a member.
	ActionManage Action = "manage"
)

var grants = map[Role][]Action{
	RoleOwner:  {ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionManage},
	RoleAdmin:  {ActionRead, ActionCreate, ActionUpdate, ActionManage},
	RoleEditor: {ActionRead, ActionCreate, ActionUpdate},
	RoleViewer: {ActionRead},
}

// Can reports whether r allows a.
func (r Role) Can(a Action) bool {
	return slices.Contains(grants[r], a)
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	_, ok := grants[r]

	return ok
}
