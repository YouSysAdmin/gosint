// Gosint, Copyright (c) 2026 YouSysAdmin

package user

import (
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/yousysadmin/gosint/internal/core/access"
	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/core/response"
	"github.com/yousysadmin/gosint/internal/core/validation"
	usermodel "github.com/yousysadmin/gosint/internal/models/user"
)

// searchLimit caps the user search, which feeds an autocomplete.
const searchLimit = 5

// Handler serves the profile and user search endpoints.
type Handler struct {
	Runtime *env.Runtime
}

// Me answers the signed-in account, with what it needs to know about
// its own sign-in.
func (h Handler) Me(c fiber.Ctx) error {
	p, err := ProfileFor(c, h.Runtime.Store, access.User(c))
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, userResponse{User: p})
}

// UpdateMe changes the signed-in account's own profile.
func (h Handler) UpdateMe(c fiber.Ctx) error {
	in, resp, ok := validation.Bind[updateProfileInput](c)
	if !ok {
		return resp
	}

	u := *access.User(c)
	if in.FirstName != nil {
		u.FirstName = *in.FirstName
	}

	if in.LastName != nil {
		u.LastName = *in.LastName
	}

	if in.AvatarURL != nil {
		if *in.AvatarURL != "" {
			if parsed, err := url.Parse(*in.AvatarURL); err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return response.BadRequestFields(c, "Avatar URL must be an http(s) URL",
					[]validation.FieldError{{Field: "avatar_url", Rule: "url", Message: "Avatar URL must be an http(s) URL"}})
			}
		}

		u.AvatarURL = *in.AvatarURL
	}

	u.UpdatedAt = time.Now().UTC()
	if err := h.Runtime.Store.User.UpdateProfile(c, &u); err != nil {
		return response.Internal(c, err)
	}

	p, err := ProfileFor(c, h.Runtime.Store, &u)
	if err != nil {
		return response.Internal(c, err)
	}

	return response.Success(c, userResponse{User: p})
}

// Search feeds the share dialog. A prefix finds the people the caller
// already shares an investigation with, a complete address finds that
// one account whoever it is, so the directory cannot be listed by
// prefixes but a new colleague can still be added.
func (h Handler) Search(c fiber.Ctx) error {
	q := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if q == "" {
		return response.BadRequest(c, "q is required")
	}

	caller := access.User(c).ID

	var found []usermodel.User
	if completeAddress(q) {
		u, err := h.Runtime.Store.User.GetByEmail(c, q)
		if err != nil {
			return response.Internal(c, err)
		}

		if u != nil && u.Active && u.ID != caller {
			found = append(found, *u)
		}
	} else {
		var err error

		found, err = h.Runtime.Store.User.SearchColleagues(c, caller, q, searchLimit)
		if err != nil {
			return response.Internal(c, err)
		}
	}

	out := make([]Profile, 0, len(found))
	for i := range found {
		out = append(out, ProfileOf(&found[i]))
	}

	return response.Success(c, usersResponse{Users: out})
}

// completeAddress tells a whole email address from a prefix of one.
func completeAddress(q string) bool {
	a, err := mail.ParseAddress(q)
	if err != nil || a.Address != q {
		return false
	}

	_, domain, _ := strings.Cut(q, "@")

	return strings.Contains(domain, ".")
}
