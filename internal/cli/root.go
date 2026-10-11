// Gosint, Copyright (c) 2026 YouSysAdmin

// Package cli wires the cobra commands the gosint binary exposes.
// Management is done via the web UI, so this surface stays small.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/yousysadmin/gosint/pkg"
)

// NewRoot builds the root command.
//
// Errors are silenced here and printed once by main.
func NewRoot() *cobra.Command {
	cobra.EnableCommandSorting = false

	root := &cobra.Command{
		Use:           pkg.AppName,
		Short:         "Self-hosted OSINT graph exploration",
		Version:       pkg.Version,
		SilenceUsage:  true,
		SilenceErrors: true,

		// Runnable so that an unknown subcommand reaches Args and comes
		// back as a typed usage error.
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.SetFlagErrorFunc(flagError)
	root.PersistentFlags().String("config", "", "config file path (yaml), defaults to ./gosint.yaml")

	root.AddGroup(
		&cobra.Group{ID: groupNode, Title: "Run a node:"},
		&cobra.Group{ID: groupInspect, Title: "Inspect this build:"},
	)

	add(root, groupNode, newServeCmd(), newVaultCmd())
	add(root, groupInspect, newVersionCmd())

	return root
}

// Help groups. cobra panics on a GroupID that is not registered on the
// parent, so NewRoot adds these before any command.
const (
	groupNode    = "node"
	groupInspect = "inspect"
)

func add(parent *cobra.Command, group string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = group
	}

	parent.AddCommand(cmds...)
}
