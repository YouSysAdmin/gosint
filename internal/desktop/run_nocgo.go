// Gosint, Copyright (c) 2026 YouSysAdmin

//go:build !cgo

package desktop

import (
	"context"
	"errors"
)

// Run needs the system web view, which only a cgo build links.
func Run(context.Context) error {
	return errors.New("gosint-desktop was built without cgo, the window needs it")
}
