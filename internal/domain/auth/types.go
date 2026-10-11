// Gosint, Copyright (c) 2026 YouSysAdmin

package auth

import (
	"time"

	"github.com/yousysadmin/gosint/internal/domain/user"
	pkmodel "github.com/yousysadmin/gosint/internal/models/passkey"
)

// registerInput is a new account. bcrypt reads at most 72 bytes, so a
// longer password would be silently truncated.
type registerInput struct {
	Email    string `json:"email"    validate:"required,email,max=254" normalize:"normalize"`
	Password string `json:"password" validate:"required,min=8,maxbytes=72"`

	// Invite is an invitation token, which admits the invited address
	// while registration is closed.
	Invite string `json:"invite,omitzero" validate:"omitempty,len=64,hexadecimal"`
}

// loginInput is a sign-in. No minimum length, so an account created
// under an older rule can still sign in.
type loginInput struct {
	Email    string `json:"email"    validate:"required,max=254" normalize:"normalize"`
	Password string `json:"password" validate:"required,maxbytes=72"`
}

// sessionResponse answers a registration or a login.
type sessionResponse struct {
	User user.Profile `json:"user"`
}

// loginResponse answers a sign-in: the account, or the second factor
// still owed with no session yet.
type loginResponse struct {
	User         *user.Profile `json:"user,omitzero"`
	SecondFactor string        `json:"second_factor,omitzero"`

	// Invite is the invitation to land on after the second factor of a
	// provider sign-in.
	Invite string `json:"invite,omitzero"`
}

// secondFactorInput is an authenticator code or a recovery code.
type secondFactorInput struct {
	Code string `json:"code" validate:"required,min=6,max=19" normalize:"trim"`
}

// resetRequestInput asks for a reset link.
type resetRequestInput struct {
	Email string `json:"email" validate:"required,email,max=254" normalize:"normalize"`
}

// resetConfirmInput redeems a link.
type resetConfirmInput struct {
	Token    string `json:"token"    validate:"required,len=64,hexadecimal"`
	Password string `json:"password" validate:"required,min=8,maxbytes=72"`
}

// changePasswordInput is the signed-in change.
type changePasswordInput struct {
	CurrentPassword string `json:"current_password" validate:"required,maxbytes=72"`
	Password        string `json:"password"         validate:"required,min=8,maxbytes=72"`
}

// reauthInput confirms the password for a sensitive change.
type reauthInput struct {
	Password string `json:"password" validate:"required,maxbytes=72"`
}

// totpCodeInput is one authenticator code.
type totpCodeInput struct {
	Code string `json:"code" validate:"required,len=6,numeric" normalize:"trim"`
}

// messageResponse answers with one line for the person.
type messageResponse struct {
	Message string `json:"message"`
}

// totpSetupResponse hands out the pending secret for the authenticator.
type totpSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURL string `json:"otpauth_url"`
}

// totpStateResponse says whether the factor is on, with the recovery
// codes when it was just turned on.
type totpStateResponse struct {
	TOTPEnabled   bool     `json:"totp_enabled"`
	RecoveryCodes []string `json:"recovery_codes,omitzero"`
}

// recoveryCodesResponse is a fresh set, shown once.
type recoveryCodesResponse struct {
	Codes []string `json:"codes"`
}

// recoveryCodesStatusResponse counts the unspent codes.
type recoveryCodesStatusResponse struct {
	Remaining int `json:"remaining"`
}

// passkeyRenameInput relabels a passkey.
type passkeyRenameInput struct {
	Name string `json:"name" validate:"required,max=60" normalize:"trim"`
}

// passkeyListResponse answers the caller's passkeys.
type passkeyListResponse struct {
	Passkeys []pkmodel.Passkey `json:"passkeys"`
}

// passkeyResponse answers an enrolment.
type passkeyResponse struct {
	Passkey pkmodel.Passkey `json:"passkey"`
}

// sessionView is one sign-in as the sessions page shows it.
type sessionView struct {
	ID        string    `json:"id"`
	UserAgent string    `json:"user_agent"`
	ClientIP  string    `json:"client_ip"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`

	// Current marks the session making the request.
	Current bool `json:"current"`
}

// sessionsResponse lists the account's sign-ins.
type sessionsResponse struct {
	Sessions []sessionView `json:"sessions"`
}

// revokedResponse counts the sessions ended.
type revokedResponse struct {
	Revoked int64 `json:"revoked"`
}

// loginProvider is one identity provider offered on the sign-in page.
type loginProvider struct {
	Slug     string `json:"slug"`
	Name     string `json:"name"`
	StartURL string `json:"start_url"`
}

// linkStartResponse is where the browser goes to link a provider.
type linkStartResponse struct {
	RedirectURL string `json:"redirect_url"`
}

// identityView is one provider linked to the caller's account.
type identityView struct {
	ID           string    `json:"id"`
	ProviderName string    `json:"provider_name"`
	ProviderSlug string    `json:"provider_slug"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"created_at"`
	LastLoginAt  time.Time `json:"last_login_at"`
}

// identitiesResponse lists the caller's linked providers.
type identitiesResponse struct {
	Identities []identityView `json:"identities"`
}

// authInfoResponse tells the sign-in page what this install offers.
type authInfoResponse struct {
	PasskeysEnabled  bool `json:"passkeys_enabled"`
	MailEnabled      bool `json:"mail_enabled"`
	RegistrationOpen bool `json:"registration_open"`

	// SetupRequired is a fresh install with no account yet, whose first
	// registration claims it as its administrator.
	SetupRequired bool            `json:"setup_required"`
	Providers     []loginProvider `json:"providers"`
}
