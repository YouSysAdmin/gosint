// Gosint, Copyright (c) 2026 YouSysAdmin

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/yousysadmin/gosint/pkg"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the binary version",
		Args:  noArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", pkg.AppName, pkg.Version)
		},
	}
}
