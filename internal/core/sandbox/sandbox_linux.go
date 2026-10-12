// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"syscall"

	"github.com/landlock-lsm/go-landlock/landlock"
	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

// Supported reports whether this node can confine a program: a kernel
// with Landlock enabled, which a container's seccomp profile may also
// hide.
func Supported() bool {
	v, err := llsys.LandlockGetABIVersion()

	return err == nil && v >= 1
}

// restrict confines this process and what it starts to p. A path that
// does not exist is skipped, a file gets file rights, a directory the
// tree beneath it.
func restrict(p Policy) error {
	if !Supported() {
		return ErrUnsupported
	}

	var rules []landlock.Rule

	add := func(paths []string, dirs, files func(...string) landlock.FSRule) {
		for _, path := range paths {
			info, err := os.Stat(path)
			if err != nil {
				continue
			}

			if info.IsDir() {
				rules = append(rules, dirs(path))
			} else {
				rules = append(rules, files(path))
			}
		}
	}

	add(p.Read, landlock.RODirs, landlock.ROFiles)
	add(p.Write, landlock.RWDirs, landlock.RWFiles)

	// The newest set the library knows, lowered to what the kernel has.
	// Supported above already refused a kernel with none.
	if err := landlock.V5.BestEffort().RestrictPaths(rules...); err != nil {
		return fmt.Errorf("landlock: %w", err)
	}

	return nil
}

func execve(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env) //nolint:gosec // the operator's own manifest, now confined
}
