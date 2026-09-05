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

const maximumPasswordBytes = 1024

func newAdminCommand() *cobra.Command {
	command := &cobra.Command{Use: "admin", Short: "Bootstrap Meridian administration"}
	command.AddCommand(newAdminCreateCommand())
	return command
}

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

func readPassword(input io.Reader, filename string) (string, error) {
	source := input
	var file *os.File
	if filename == "" {
		if source == nil {
			return "", errors.New("administrator password input is unavailable")
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
	data, err := io.ReadAll(io.LimitReader(source, maximumPasswordBytes+3))
	if err != nil {
		return "", fmt.Errorf("read administrator password: %w", err)
	}
	password := string(data)
	password, _ = strings.CutSuffix(password, "\n")
	password, _ = strings.CutSuffix(password, "\r")
	if len(password) > maximumPasswordBytes {
		return "", errors.New("administrator password exceeds 1024 bytes")
	}
	if password == "" {
		return "", errors.New("administrator password is required on stdin or --password-file")
	}
	return password, nil
}
