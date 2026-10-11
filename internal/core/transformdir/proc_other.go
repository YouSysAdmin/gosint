// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build !unix

package transformdir

import "os/exec"

// ownGroup has no process groups here, the script alone is stopped.
func ownGroup(*exec.Cmd) {}
