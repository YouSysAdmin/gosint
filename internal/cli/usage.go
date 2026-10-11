// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// UsageError marks a failure the caller could have avoided: an unknown
// flag or command, a stray argument, a flag value outside its set.
// main exits 2 for these and 1 for everything else.
type UsageError struct {
	Err error
}

// Error renders the failure for a log or a caller.
func (e *UsageError) Error() string { return e.Err.Error() }

// Unwrap returns the underlying error, for errors.Is and errors.As.
func (e *UsageError) Unwrap() error { return e.Err }

func usage(format string, args ...any) error {
	return &UsageError{Err: fmt.Errorf(format, args...)}
}

// ExitCode maps a command's error to the process exit status.
func ExitCode(err error) int {
	switch {
	case err == nil:
		return 0
	case isUsage(err):
		return 2
	default:
		return 1
	}
}

func isUsage(err error) bool {
	_, ok := errors.AsType[*UsageError](err)

	return ok
}

// noArgs is cobra.NoArgs typed as a usage error.
func noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return &UsageError{Err: err}
	}

	return nil
}

func flagError(_ *cobra.Command, err error) error {
	return &UsageError{Err: err}
}
