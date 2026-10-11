// Gosint, Copyright (c) 2026 YouSysAdmin

// Package transforms is every built-in transform. They live in one
// package per input type (domain,
// ip, email, website...), each a small struct implementing
// transform.Transform and listed by that package's All, so adding one
// touches its package alone and nothing registers by side effect. The
// internal packages hold what several share: HTTP and page reading,
// the API clients, the WHOIS contact writer.
//
// An HTTP transform keeps its base URL in a field, so a test points it
// at a local server.
package transforms

import (
	"slices"

	"github.com/yousysadmin/gosint/internal/core/tool"
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/asn"
	"github.com/yousysadmin/gosint/internal/transforms/automation"
	"github.com/yousysadmin/gosint/internal/transforms/cidr"
	"github.com/yousysadmin/gosint/internal/transforms/crypto"
	"github.com/yousysadmin/gosint/internal/transforms/cve"
	"github.com/yousysadmin/gosint/internal/transforms/domain"
	"github.com/yousysadmin/gosint/internal/transforms/email"
	"github.com/yousysadmin/gosint/internal/transforms/expand"
	"github.com/yousysadmin/gosint/internal/transforms/filter"
	"github.com/yousysadmin/gosint/internal/transforms/finding"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/nmap"
	"github.com/yousysadmin/gosint/internal/transforms/ip"
	"github.com/yousysadmin/gosint/internal/transforms/location"
	"github.com/yousysadmin/gosint/internal/transforms/org"
	"github.com/yousysadmin/gosint/internal/transforms/person"
	"github.com/yousysadmin/gosint/internal/transforms/phone"
	"github.com/yousysadmin/gosint/internal/transforms/username"
	"github.com/yousysadmin/gosint/internal/transforms/vulnerability"
	"github.com/yousysadmin/gosint/internal/transforms/website"
)

// Tools is every tool the built-in transforms use, set by an
// administrator under Tools.
func Tools() []tool.Tool {
	return []tool.Tool{tool.Web{}, nmap.Tool{}}
}

// All is every built-in transform, each pointed at its public source.
func All() []transform.Transform {
	return slices.Concat(
		domain.All(), ip.All(), email.All(), website.All(),
		org.All(), person.All(), phone.All(), username.All(), crypto.All(), cve.All(), vulnerability.All(), expand.All(), finding.All(),
		asn.All(), cidr.All(), location.All(), automation.All(), filter.All(),
	)
}
