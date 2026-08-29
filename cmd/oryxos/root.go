package main

import (
	"errors"
	"io"
	"os"

	"github.com/Karlsk/oryxos-go/internal/app"
	"github.com/spf13/cobra"
)

// ErrNotImplemented marks a declared CLI leaf whose business behavior is deferred.
var ErrNotImplemented = errors.New("not implemented")

// CommandDependencies supplies process-boundary inputs and deterministic test seams.
type CommandDependencies struct {
	WorkspaceRoot     string
	FoundationOptions app.FoundationOptions
	Output            io.Writer
}

// NewRootCommand constructs the complete, bounded OryxOS command tree.
func NewRootCommand(dependencies CommandDependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "oryxos",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	if dependencies.Output != nil {
		root.SetOut(dependencies.Output)
	} else {
		root.SetOut(os.Stdout)
	}
	root.SetErr(os.Stderr)

	root.AddCommand(newInitCommand(dependencies))
	root.AddCommand(newStatusCommand(dependencies))
	root.AddCommand(newChatCommand())
	root.AddCommand(newServeCommand(dependencies))
	root.AddCommand(newDeferredCommand("gateway", nil))
	root.AddCommand(newProfileCommand())
	root.AddCommand(newDeferredGroup("provider", "list"))
	root.AddCommand(newDeferredGroup("tool", "list"))
	root.AddCommand(newDeferredGroup("session", "list"))
	return root
}
