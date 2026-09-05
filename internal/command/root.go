// Package command defines the meridian command tree.
package command

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

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
	root.AddCommand(newServeCommand(), newVersionCommand())
	return root
}

func Execute(ctx context.Context, stdout, stderr io.Writer) error {
	root := New()
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.ExecuteContext(ctx)
}
