// Gosint, Copyright (c) 2026 YouSysAdmin

package scan

import scanmodel "github.com/yousysadmin/gosint/internal/models/scan"

// scanResponse answers a single scan.
type scanResponse struct {
	Scan scanmodel.Scan `json:"scan"`
}

// scansResponse answers a sketch's scans.
type scansResponse struct {
	Scans []scanmodel.Scan `json:"scans"`
}
