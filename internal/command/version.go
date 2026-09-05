package command

import (
	"fmt"

	"github.com/meridian-labs/meridian/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the Meridian build version",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(command.OutOrStdout(), "meridian %s (%s)\n", buildinfo.CurrentVersion(), buildinfo.CurrentCommit())
			return err
		},
	}
}
