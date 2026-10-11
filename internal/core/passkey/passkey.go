// Gosint, Copyright (c) 2026 YouSysAdmin

// Package passkey wraps go-webauthn for passwordless sign-in (WebAuthn,
// FIDO2). Ported from mailyard.
//
// The relying party is built per request from the browser's Origin
// rather than from the configured public URL. The Origin is what the
// browser signs into the client data, so deriving the RP from it keeps
// the check equal to what the authenticator bound, on localhost and
// behind a proxy alike.
package passkey

import (
	"bytes"
	"encoding/json/v2"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// displayName is what the authenticator shows when it asks the person
// to confirm. The product, not the install.
const displayName = "Gosint"

// Re-exported so callers and the store never import go-webauthn.
type (
	Credential          = webauthn.Credential
	SessionData         = webauthn.SessionData
	CredentialCreation  = protocol.CredentialCreation
	CredentialAssertion = protocol.CredentialAssertion
)

// EncodeCredential packs a credential for its opaque JSON column.
func EncodeCredential(c *Credential) (string, error) {
	b, err := json.Marshal(c)

	return string(b), err
}

// DecodeCredential reads a stored credential back. An error rather
// than a zero value, a credential that will not decode authenticates
// nobody.
func DecodeCredential(raw string) (*Credential, error) {
	var c Credential
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil, err
	}

	return &c, nil
}

// Service is a relying party bound to one (rpID, origin).
type Service struct{ wa *webauthn.WebAuthn }

// ceremonyTimeout is how long a challenge stays answerable.
const ceremonyTimeout = 5 * time.Minute

// New builds the RP for rpID (the host, no scheme or port) and origin
// (the exact scheme://host[:port] the browser used).
func New(rpID, origin string) (*Service, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: displayName,
		RPOrigins:     []string{origin},
		// Enforced here, not only advertised to the browser, so a
		// captured challenge and its assertion expire server side.
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout, TimeoutUVD: ceremonyTimeout},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ceremonyTimeout, TimeoutUVD: ceremonyTimeout},
		},
	})
	if err != nil {
		return nil, err
	}

	return &Service{wa: wa}, nil
}

// User adapts an account to webauthn.User. The handle is the account
// id, which is opaque already.
type User struct {
	Handle  []byte
	Name    string
	Display string
	Creds   []Credential
}

// WebAuthnID implements webauthn.User.
func (u *User) WebAuthnID() []byte { return u.Handle }

// WebAuthnName implements webauthn.User.
func (u *User) WebAuthnName() string { return u.Name }

// WebAuthnDisplayName implements webauthn.User.
func (u *User) WebAuthnDisplayName() string {
	if u.Display != "" {
		return u.Display
	}

	return u.Name
}

// WebAuthnCredentials implements webauthn.User.
func (u *User) WebAuthnCredentials() []Credential { return u.Creds }

// BeginRegistration starts enrolment of a discoverable passkey,
// excluding the credentials already on the account. User verification
// is required: a passkey signs in on its own, so it has to be backed
// by a biometric or a PIN, and the library only enforces the flag
// when the ceremony asked for it.
func (s *Service) BeginRegistration(u *User) (*CredentialCreation, *SessionData, error) {
	exclude := make([]protocol.CredentialDescriptor, 0, len(u.Creds))
	for i := range u.Creds {
		exclude = append(exclude, u.Creds[i].Descriptor())
	}

	return s.wa.BeginRegistration(u,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithExclusions(exclude),
	)
}

// FinishRegistration verifies the attestation in body.
func (s *Service) FinishRegistration(u *User, sess SessionData, body []byte) (*Credential, error) {
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	return s.wa.CreateCredential(u, sess, parsed)
}

// BeginLogin starts a usernameless assertion, so the browser shows its
// passkey picker and no email is typed.
func (s *Service) BeginLogin() (*CredentialAssertion, *SessionData, error) {
	return s.wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
}

// FinishLogin verifies the assertion in body. resolve maps the
// authenticator's (rawID, userHandle) to the owning account, which
// comes back beside the validated credential so the caller can mint a
// session and store the new sign count.
func (s *Service) FinishLogin(resolve func(rawID, userHandle []byte) (*User, error), sess SessionData, body []byte) (*Credential, *User, error) {
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}

	var matched *User
	handler := func(rawID, userHandle []byte) (webauthn.User, error) {
		u, err := resolve(rawID, userHandle)
		if err != nil {
			return nil, err
		}

		matched = u

		return u, nil
	}
	cred, err := s.wa.ValidateDiscoverableLogin(handler, sess, parsed)

	return cred, matched, err
}
