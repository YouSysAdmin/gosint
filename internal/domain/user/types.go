// Gosint, Copyright (c) 2026 YouSysAdmin

package user

import (
	"context"
	"time"

	"github.com/yousysadmin/gosint/internal/domain/store"
	invmodel "github.com/yousysadmin/gosint/internal/models/investigation"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// updateProfileInput is a partial profile update.
type updateProfileInput struct {
	FirstName *string `json:"first_name" validate:"omitnil,max=100" normalize:"trim"`
	LastName  *string `json:"last_name"  validate:"omitnil,max=100" normalize:"trim"`
	// AvatarURL is checked in the handler, an empty one clears it, which
	// the url rule would refuse.
	AvatarURL *string `json:"avatar_url" validate:"omitnil,max=2048" normalize:"trim"`
}

// Profile is an account as other people see it, plus what its owner
// needs to know about how it signs in.
type Profile struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	AvatarURL string `json:"avatar_url"`

	// Admin administers accounts. AccountType is local or OIDC,
	// TOTPEnabled and Passkeys say what second factors the account
	// holds, so the profile page draws the right controls.
	Admin       bool `json:"admin"`
	AccountType int  `json:"account_type"`
	TOTPEnabled bool `json:"totp_enabled"`
	Passkeys    int  `json:"passkeys"`
}

// ProfileOf is the view of u without its passkey count, for lists.
func ProfileOf(u *usermodel.User) Profile {
	return Profile{ID: u.ID, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName, AvatarURL: u.AvatarURL,
		Admin: u.Admin, AccountType: u.AccountType, TOTPEnabled: u.TOTPEnabled}
}

// ProfileFor is the owner's own view, with the passkey count read.
func ProfileFor(ctx context.Context, st *store.Store, u *usermodel.User) (Profile, error) {
	p := ProfileOf(u)
	n, err := st.Passkey.CountForUser(ctx, u.ID)
	if err != nil {
		return p, err
	}

	p.Passkeys = n

	return p, nil
}

// userResponse answers the profile endpoints.
type userResponse struct {
	User Profile `json:"user"`
}

// usersResponse answers the user search.
type usersResponse struct {
	Users []Profile `json:"users"`
}

// invmodelInvestigation is the investigation record, named here so the
// admin answers can carry what an account owns.
type invmodelInvestigation = invmodel.Investigation

// UserFilter narrows the admin list.
type UserFilter = usermodel.Filter

// AdminUser is an account as the admin panel lists it.
type AdminUser struct {
	Profile

	Active    bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// adminCreateInput adds an account. A password is optional, the person
// gets a reset link otherwise.
type adminCreateInput struct {
	Email    string `json:"email"    validate:"required,email,max=254" normalize:"normalize"`
	Password string `json:"password" validate:"omitempty,min=8,maxbytes=72"`
	Admin    bool   `json:"admin"`
}

// adminUpdateInput changes the flags or sets a password, each optional.
type adminUpdateInput struct {
	Admin    *bool  `json:"admin"`
	Disabled *bool  `json:"disabled"`
	Password string `json:"password" validate:"omitempty,min=8,maxbytes=72"`
}

// adminUsersResponse lists accounts.
type adminUsersResponse struct {
	Users []AdminUser `json:"users"`
	Total int         `json:"total"`
}

// adminUserResponse answers one account with what it owns.
type adminUserResponse struct {
	User AdminUser               `json:"user"`
	Owns []invmodelInvestigation `json:"owns"`
}

// resetLinkResponse hands a reset link to the administrator.
type resetLinkResponse struct {
	Link    string `json:"link"`
	Emailed bool   `json:"emailed"`
}

// removedResponse counts removed passkeys.
type removedResponse struct {
	Removed int `json:"removed"`
}

// revokedResponse counts ended sessions.
type revokedResponse struct {
	Revoked int64 `json:"revoked"`
}
