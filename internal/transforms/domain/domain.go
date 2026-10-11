// Gosint, Copyright (c) 2026 YouSysAdmin

// Package domain holds the transforms that start from a domain name.
package domain

import (
	"github.com/yousysadmin/gosint/internal/core/transform"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/crtsh"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/dehashed"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/ripestat"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/subfinder"
	"github.com/yousysadmin/gosint/internal/transforms/internal/sources/whoxy"
)

// All is the package's transforms, pointed at their public sources.
func All() []transform.Transform {
	return []transform.Transform{
		DomainToURLScan{BaseURL: URLScanURL},
		DomainToVirusTotal{BaseURL: VirusTotalURL},
		DomainToSecurityTrails{BaseURL: SecurityTrailsURL},
		DomainToHunter{BaseURL: HunterURL},
		DomainToDNS{},
		DomainToIP{},
		DomainToSubdomains{BaseURL: crtsh.URL, Passive: subfinder.Enumerate},
		DomainToRootDomain{},
		DomainToWebsite{},
		DomainToTLS{},
		DomainToWhois{},
		DomainToHistory{Client: whoxy.Public},
		DomainToWhoisHistory{BaseURL: WhoisXMLURL},
		DomainToASN{Client: ripestat.Public},
		DomainToDehashed{Client: dehashed.Public},
	}
}
