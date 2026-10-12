// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build unix

package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrRunning says another instance holds the data directory.
var ErrRunning = errors.New("gosint is already running")

// Lock takes the data directory for this process. The lock goes with
// the process, so a crash leaves nothing to clean.
func Lock(dir string) (release func(), err error) {
	f, err := os.OpenFile(filepath.Join(dir, "desktop.lock"), os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // a fixed name in the data directory
	if err != nil {
		return nil, err
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()

		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunning
		}

		return nil, err
	}

	return func() { _ = f.Close() }, nil
}
