// Package command 定义 meridian 命令树。
package command

import (
	"context"
	"io"

	"github.com/spf13/cobra"
)

// New 构造 Meridian 根命令及其全部受支持的子命令。
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
	root.AddCommand(newAdminCommand(), newMigrateCommand(), newServeCommand(), newVersionCommand())
	return root
}

// Execute 使用调用方持有的 I/O 流运行 Meridian 命令树。
func Execute(ctx context.Context, stdout, stderr io.Writer) error {
	root := New()
	root.SetOut(stdout)
	root.SetErr(stderr)
	return root.ExecuteContext(ctx)
}
