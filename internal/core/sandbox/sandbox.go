// Gosint, Copyright (c) 2026 YouSysAdmin

// Package sandbox confines a program the server starts, a transform
// file's script, to the paths it needs. The server never restricts
// itself: it starts its own binary with Arg, which restricts that new
// process and then replaces it with the program. Linux only, through
// Landlock, every other system reports Supported false.
package sandbox

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Arg is the first argument that turns the binary into the helper.
// Both mains hand it to Main before anything else.
const Arg = "_sandbox"

// ErrUnsupported is a node that cannot confine a program.
var ErrUnsupported = errors.New("the script sandbox needs Linux with Landlock (5.13 or newer)")

// Policy is what a confined program may touch. Everything else is
// refused, /proc included, so the server's own environment is out of
// reach.
type Policy struct {
	// Read may be read and executed, a directory with all beneath it.
	Read []string `json:"read"`

	// Write may be read, written and created, the run's own scratch
	// directory and what the administrator adds.
	Write []string `json:"write"`
}

// systemRead is what an interpreter and its libraries need to start:
// the system trees, and single entries of /etc rather than all of it,
// where a server's own config may live.
var systemRead = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32", "/opt",
	"/etc/ssl", "/etc/ca-certificates", "/etc/ca-certificates.conf", "/etc/pki",
	"/etc/resolv.conf", "/etc/hosts", "/etc/host.conf", "/etc/nsswitch.conf", "/etc/gai.conf",
	"/etc/services", "/etc/protocols", "/etc/localtime", "/etc/timezone",
	"/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d", "/etc/alternatives",
	"/etc/passwd", "/etc/group", "/etc/mime.types", "/etc/os-release",
	"/etc/python3", "/etc/pip.conf", "/etc/ssh/ssh_config", "/etc/gitconfig",
	"/dev/urandom", "/dev/random", "/dev/zero",
	// Resolves to the helper's own entry, which the program keeps
	// after the exec, never another process's.
	"/proc/self",
}

// systemWrite is what any program writes to.
var systemWrite = []string{"/dev/null"}

// Default is the policy for a script in dir with scratch as its only
// writable place, plus the administrator's extra paths.
func Default(dir, scratch string, read, write []string) Policy {
	p := Policy{
		Read:  append(append([]string{}, systemRead...), dir),
		Write: append(append([]string{}, systemWrite...), scratch),
	}

	p.Read = append(p.Read, read...)
	p.Write = append(p.Write, write...)

	return p
}

// Command is argv run confined by p: this binary started as the helper.
func Command(ctx context.Context, p Policy, argv []string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("sandbox: nothing to run")
	}

	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("sandbox: this binary: %w", err)
	}

	doc, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	args := append([]string{Arg, string(doc), "--"}, argv...)

	return exec.CommandContext(ctx, self, args...), nil //nolint:gosec // this binary, the operator's own manifest after it
}

// Main runs the helper when the process was started with Arg and does
// not return then. Otherwise it returns at once.
func Main() {
	if len(os.Args) < 2 || os.Args[1] != Arg {
		return
	}

	err := run(os.Args[2:])
	fmt.Fprintln(os.Stderr, "sandbox:", err)
	os.Exit(126)
}

// run reads the policy, restricts this process and replaces it with
// the program. It returns only on a failure.
func run(args []string) error {
	if len(args) < 3 || args[1] != "--" {
		return errors.New("usage: " + Arg + " <policy> -- <program> [args]")
	}

	var p Policy
	if err := json.Unmarshal([]byte(args[0]), &p); err != nil {
		return fmt.Errorf("policy: %w", err)
	}

	argv := args[2:]

	// Looked up before the restriction, which may hide the PATH entries
	// that are not allowed.
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}

	if path, err = filepath.Abs(path); err != nil {
		return err
	}

	if err := restrict(p); err != nil {
		return err
	}

	return execve(path, argv, os.Environ())
}
