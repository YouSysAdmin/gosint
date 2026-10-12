// Gosint, Copyright (c) 2026 YouSysAdmin

// Command gosint is the single binary this project ships: the API, the
// console and the enrichment workers. Which of those a process runs is
// decided by the subcommand - see internal/cli.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/yousysadmin/gosint/internal/cli"
	"github.com/yousysadmin/gosint/internal/core/sandbox"
	"github.com/yousysadmin/gosint/pkg"
)

func main() {
	// A transform file's script is started through this binary, which
	// confines it and does not come back.
	sandbox.Main()

	// Sets the UTC time zone and ignores server time settings.
	time.Local = time.UTC

	err := cli.NewRoot().Execute()
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "%s: %v\n", pkg.AppName, err)
	os.Exit(cli.ExitCode(err))
}
