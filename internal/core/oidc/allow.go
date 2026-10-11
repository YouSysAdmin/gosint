// Gosint, Copyright (c) 2026 YouSysAdmin

package oidc

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Admit checks the allow lists against the claims: email_verified when
// required, then allowed_emails (an exact match admits on its own),
// allowed_domains, allowed_groups. All empty admits, the IdP is the
// gate. The error is for the log, the person sees a generic refusal.
func (p *Provider) Admit(c *Claims) error {
	cfg := p.cfg
	email := strings.ToLower(strings.TrimSpace(c.Email))
	if cfg.RequireEmailVerified && !c.EmailVerified {
		return errors.New("email_verified is false")
	}

	if len(cfg.AllowedEmails) > 0 {
		if email == "" {
			return errors.New("allowed_emails is set but the token has no email claim")
		}

		if !slices.Contains(cfg.AllowedEmails, email) {
			return fmt.Errorf("email %q is not on allowed_emails", email)
		}

		return nil
	}

	if len(cfg.AllowedDomains) > 0 {
		if email == "" {
			return errors.New("allowed_domains is set but the token has no email claim")
		}

		_, host, ok := strings.Cut(email, "@")
		if !ok || host == "" {
			return fmt.Errorf("malformed email %q", email)
		}

		if !slices.Contains(cfg.AllowedDomains, host) {
			return fmt.Errorf("domain %q is not on allowed_domains", host)
		}
	}

	if len(cfg.AllowedGroups) > 0 {
		groups := extractGroups(c.Raw, cfg.GroupsClaim)
		if len(groups) == 0 {
			return fmt.Errorf("allowed_groups is set but the claim %q is empty or missing", cfg.GroupsClaim)
		}

		matched := slices.ContainsFunc(groups, func(g string) bool {
			return slices.Contains(cfg.AllowedGroups, strings.ToLower(strings.TrimSpace(g)))
		})
		if !matched {
			return fmt.Errorf("none of the groups %v is on allowed_groups", groups)
		}
	}

	return nil
}

// extractGroups reads the claim in the shapes IdPs use: a list of
// strings, a list of anything, one string.
func extractGroups(raw map[string]any, claim string) []string {
	if claim == "" {
		return nil
	}

	switch t := raw[claim].(type) {
	case []string:
		return t
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}

		return out
	}

	return nil
}
