// Gosint, Copyright (c) 2026 YouSysAdmin

// Package investigation persists investigations and their members and
// serves the investigation and membership endpoints.
//
// Access is decided per investigation by the member's role. A caller
// who is not a member gets a 404, so an investigation they cannot see
// looks like one that does not exist.
package investigation
