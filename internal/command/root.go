// Package command defines the meridian command tree.
package command

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// New constructs the Meridian root command and all supported subcommands.
func New() *cobra.Command {
	root := &cobra.Command{
		Use:           "meridian",
		Short:         "Meridian asset platform",
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.AddCommand(newMigrateCommand(), newServeCommand(), newVersionCommand())
	return root
}

// Execute runs the Meridian command tree with caller-owned I/O streams.
func Execute(ctx context.Context, stdout, stderr io.Writer) error {
	root := New()
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.ExecuteContext(ctx)
}
