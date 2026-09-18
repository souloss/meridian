package command

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/spf13/cobra"
)

// 管理员密码域常量。
const (
	// maximumPasswordBytes 是平台管理员密码允许的最大字节长度。
	maximumPasswordBytes = 1024
	// passwordReadSlack 是读取密码时为探测超长而多读取的字节数。
	passwordReadSlack = 3
)

// 管理员命令包内哨兵错误。
var (
	// errAdminPasswordInputUnavailable 表示无法从任何来源读取密码。
	errAdminPasswordInputUnavailable = errors.New("administrator password input is unavailable")
	// errAdminPasswordTooLong 表示密码超过允许的最大字节长度。
	errAdminPasswordTooLong = errors.New("administrator password exceeds 1024 bytes")
	// errAdminPasswordRequired 表示 stdin 或 --password-file 均未提供密码。
	errAdminPasswordRequired = errors.New("administrator password is required on stdin or --password-file")
)

// newAdminCommand 构造平台管理子命令树。
func newAdminCommand() *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Bootstrap Meridian administration"}
	command.AddCommand(newAdminCreateCommand())
	return command
}

// newAdminCreateCommand 构造创建或晋升平台管理员的子命令。
func newAdminCreateCommand() *cobra.Command {
	var databaseURL string
	var username string
	var displayName string
	var passwordFile string

	command := &cobra.Command{
		Use:   "create",
		Short: "Create or promote a platform administrator",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) (err error) {
			password, err := readPassword(command.InOrStdin(), passwordFile)
			if err != nil {
				return err
			}
			if displayName == "" {
				displayName = username
			}
			db, err := openDatabase(command.Context(), databaseURL, command.ErrOrStderr())
			if err != nil {
				return err
			}
			defer closeDatabase(db, &err)
			if err := db.MigrateUp(command.Context()); err != nil {
				return err
			}
			user, err := service.BootstrapPlatformAdmin(command.Context(), repository.NewIdentityStore(db.Pool), service.CreateUserInput{
				Username: username, Password: password, DisplayName: displayName,
			}, time.Now().UTC())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "platform administrator %s (%s) is ready\n", user.Username, user.ID)
			return err
		},
	}
	command.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MERIDIAN_DATABASE_URL"), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	command.Flags().StringVar(&username, "username", "", "local username")
	command.Flags().StringVar(&displayName, "display-name", "", "display name (defaults to username)")
	command.Flags().StringVar(&passwordFile, "password-file", "", "read the password from a file instead of stdin")
	_ = command.MarkFlagRequired("username")
	return command
}

// readPassword 从 stdin 或密码文件读取平台管理员密码，并执行长度与存在性校验。
func readPassword(input io.Reader, filename string) (string, error) {
	source := input
	var file *os.File
	if filename == "" {
		if source == nil {
			return "", errAdminPasswordInputUnavailable
		}
	} else {
		var err error
		file, err = os.Open(filename)
		if err != nil {
			return "", fmt.Errorf("open administrator password: %w", err)
		}
		defer file.Close()
		source = file
	}
	data, err := io.ReadAll(io.LimitReader(source, maximumPasswordBytes+passwordReadSlack))
	if err != nil {
		return "", fmt.Errorf("read administrator password: %w", err)
	}
	password := string(data)
	password, _ = strings.CutSuffix(password, "\n")
	password, _ = strings.CutSuffix(password, "\r")
	if len(password) > maximumPasswordBytes {
		return "", errAdminPasswordTooLong
	}
	if password == "" {
		return "", errAdminPasswordRequired
	}
	return password, nil
}
