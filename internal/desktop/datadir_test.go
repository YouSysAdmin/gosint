// Gosint, Copyright (c) 2026 YouSysAdmin

package desktop_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/yousysadmin/gosint/internal/core/env"
	"github.com/yousysadmin/gosint/internal/desktop"
)

// The console's browser storage is keyed by origin, so a second launch
// must come back on the same port, and on another one when it is taken.
func TestTheAppKeepsItsPort(t *testing.T) {
	dir := t.TempDir()

	ln, err := desktop.Listen(dir)
	if err != nil {
		t.Fatal(err)
	}

	first := ln.Addr().(*net.TCPAddr).Port

	_ = ln.Close()

	ln, err = desktop.Listen(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := ln.Addr().(*net.TCPAddr).Port; got != first {
		t.Errorf("second launch on port %d, want %d", got, first)
	}

	// Held by someone else now.
	other, err := desktop.Listen(dir)
	if err != nil {
		t.Fatal(err)
	}

	defer other.Close() //nolint:errcheck // test cleanup
	defer ln.Close()    //nolint:errcheck // test cleanup

	moved := other.Addr().(*net.TCPAddr).Port
	if moved == first {
		t.Fatal("a taken port was bound twice")
	}

	_ = other.Close()
	_ = ln.Close()

	again, err := desktop.Listen(dir)
	if err != nil {
		t.Fatal(err)
	}

	defer again.Close() //nolint:errcheck // test cleanup

	if got := again.Addr().(*net.TCPAddr).Port; got != moved {
		t.Errorf("the new port was not remembered, got %d, want %d", got, moved)
	}
}

func TestOneInstancePerDataDirectory(t *testing.T) {
	dir := t.TempDir()

	release, err := desktop.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := desktop.Lock(dir); !errors.Is(err, desktop.ErrRunning) {
		t.Errorf("second lock: %v, want ErrRunning", err)
	}

	release()

	release, err = desktop.Lock(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}

	release()
}

func TestTheConfigFileIsCreatedOnce(t *testing.T) {
	dir := t.TempDir()

	path, err := desktop.ConfigFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("logging:\n  level: debug\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := desktop.ConfigFile(dir); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path) //nolint:gosec // the test's temp directory
	if err != nil {
		t.Fatal(err)
	}

	if string(b) != "logging:\n  level: debug\n" {
		t.Errorf("an existing config was rewritten: %q", b)
	}
}

// A first launch reads the stub it wrote and keeps everything in the
// data directory.
func TestAFirstLaunchKeepsItsDataTogether(t *testing.T) {
	dir := t.TempDir()

	path, err := desktop.ConfigFile(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := env.Load(path, desktop.Defaults(dir))
	if err != nil {
		t.Fatalf("the stub does not load: %v", err)
	}

	if cfg.Database.DSN != filepath.Join(dir, "gosint.db") {
		t.Errorf("dsn = %q", cfg.Database.DSN)
	}

	if got := cfg.TransformsDir(); got != filepath.Join(dir, "transforms") {
		t.Errorf("transforms dir = %q", got)
	}
}
