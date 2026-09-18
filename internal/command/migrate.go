package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// newMigrateCommand 构造管理 Meridian 数据库 schema 的子命令树。
func newMigrateCommand() *cobra.Command {
	var databaseURL string

	command := &cobra.Command{
		Use:   "migrate",
		Short: "Manage the Meridian database schema",
	}
	command.PersistentFlags().StringVar(&databaseURL, "database-url", os.Getenv("MERIDIAN_DATABASE_URL"), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	command.AddCommand(newMigrateUpCommand(&databaseURL), newMigrateDownCommand(&databaseURL), newMigrateStatusCommand(&databaseURL))
	return command
}

// newMigrateUpCommand 构造应用全部待执行迁移的子命令。
func newMigrateUpCommand(databaseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Apply all pending River and Meridian migrations",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			db, err := openDatabase(command.Context(), *databaseURL, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer closeDatabase(db, &err)
			return db.MigrateUp(command.Context())
		},
	}
}

// newMigrateDownCommand 构造回滚显式数量迁移步骤的子命令。
func newMigrateDownCommand(databaseURL *string) *cobra.Command {
	var steps int

	command := &cobra.Command{
		Use:   "down",
		Short: "Roll back an explicit number of migration steps",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			db, err := openDatabase(command.Context(), *databaseURL, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer closeDatabase(db, &err)
			return db.MigrateDown(command.Context(), steps)
		},
	}
	command.Flags().IntVar(&steps, "steps", 0, "number of migrations to roll back")
	_ = command.MarkFlagRequired("steps")
	return command
}

// newMigrateStatusCommand 构造打印已应用迁移版本的子命令。
func newMigrateStatusCommand(databaseURL *string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print applied Meridian and River migration versions",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			db, err := openDatabase(command.Context(), *databaseURL, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer closeDatabase(db, &err)

			status, err := db.MigrationStatus(command.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "application=%d river=%d\n", status.ApplicationVersion, len(status.RiverVersions))
			return err
		},
	}
}
