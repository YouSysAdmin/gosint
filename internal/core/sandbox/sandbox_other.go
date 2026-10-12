// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build !linux

package sandbox

// Supported is false: only Linux has Landlock.
func Supported() bool { return false }

func restrict(Policy) error { return ErrUnsupported }

func execve(string, []string, []string) error { return ErrUnsupported }
