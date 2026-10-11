// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build unix

package transformdir

import (
	"os/exec"
	"syscall"
)

// ownGroup runs the script in its own process group and stops the whole
// group at the timeout, so a child it spawned does not outlive it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
