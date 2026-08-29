package main

import (
	"fmt"
	"path/filepath"

	"github.com/Karlsk/oryxos-go/internal/app"
	"github.com/spf13/cobra"
)

func newInitCommand(dependencies CommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:  "init",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return InitializeWorkspace(workspaceRoot(dependencies), cmd.OutOrStdout())
		},
	}
}

func newStatusCommand(dependencies CommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:  "status",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			state := WorkspaceStatus(workspaceRoot(dependencies))
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "mode: foundation\nworkspace: %s\n", state)
			return err
		},
	}
}

func newServeCommand(dependencies CommandDependencies) *cobra.Command {
	return &cobra.Command{
		Use:  "serve",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options := dependencies.FoundationOptions
			options.ServerYAML = nil
			application, err := app.NewFoundation(options)
			if err != nil {
				return err
			}
			return application.Run(cmd.Context())
		},
	}
}

func newChatCommand() *cobra.Command {
	command := newDeferredCommand("chat [--profile <name>]", cobra.NoArgs)
	command.Flags().String("profile", "", "profile name")
	return command
}

func newProfileCommand() *cobra.Command {
	command := &cobra.Command{Use: "profile"}
	command.AddCommand(
		newDeferredCommand("list", cobra.NoArgs),
		newDeferredCommand("create <name>", cobra.ExactArgs(1)),
		newDeferredCommand("show <name>", cobra.ExactArgs(1)),
		newDeferredCommand("delete <name>", cobra.ExactArgs(1)),
	)
	return command
}

func newDeferredGroup(group, leaf string) *cobra.Command {
	command := &cobra.Command{Use: group}
	command.AddCommand(newDeferredCommand(leaf, cobra.NoArgs))
	return command
}

func newDeferredCommand(use string, arguments cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:  use,
		Args: arguments,
		RunE: func(*cobra.Command, []string) error { return ErrNotImplemented },
	}
}

func workspaceRoot(dependencies CommandDependencies) string {
	if dependencies.WorkspaceRoot == "" {
		return "."
	}
	return filepath.Clean(dependencies.WorkspaceRoot)
}
