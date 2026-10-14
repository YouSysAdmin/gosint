// Gosint, Copyright (c) 2026 YouSysAdmin

// Command gosint-desktop runs a gosint node on the loopback and shows
// its console in a native window. It needs cgo, the server binary
// does not.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yousysadmin/gosint/internal/core/sandbox"
	"github.com/yousysadmin/gosint/internal/desktop"
)

func main() {
	// A transform file's script is started through this binary, which
	// confines it and does not come back.
	sandbox.Main()

	// Sets the UTC time zone and ignores the machine's settings.
	time.Local = time.UTC

	// Most errors were shown in a dialog already, a build without cgo
	// has no window to show one in.
	if err := desktop.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "gosint-desktop:", err)
		os.Exit(1)
	}
}
