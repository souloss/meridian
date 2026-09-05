package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/meridian-labs/meridian/internal/handler"
	"github.com/meridian-labs/meridian/internal/repository"
	"github.com/meridian-labs/meridian/internal/service"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var addr string
	var databaseURL string
	var insecureCookies bool

	command := &cobra.Command{
		Use:   "serve",
		Short: "Start the Meridian HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runServer(command.Context(), addr, databaseURL, os.Getenv("MERIDIAN_TOKEN_PEPPER"), !insecureCookies, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	command.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MERIDIAN_DATABASE_URL"), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	command.Flags().BoolVar(&insecureCookies, "insecure-cookies", false, "allow session cookies over HTTP for loopback development")
	return command
}

func runServer(ctx context.Context, addr, databaseURL, encodedPepper string, secureCookies bool, output io.Writer) (err error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	if !secureCookies && !isLoopbackAddress(addr) {
		return errors.New("--insecure-cookies requires an explicit loopback --addr")
	}
	digester, err := service.NewTokenDigester(encodedPepper)
	if err != nil {
		return err
	}
	db, err := openDatabase(ctx, databaseURL, output)
	if err != nil {
		return err
	}
	defer closeDatabase(db, &err)
	if err := db.MigrateUp(ctx); err != nil {
		return err
	}

	server := &http.Server{
		Addr: addr,
		Handler: handler.NewWithIdentity(
			service.NewIdentity(repository.NewIdentityStore(db.Pool), digester),
			secureCookies,
		).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("meridian listening", "addr", addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server stopped: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http server shutdown failed: %w", err)
		}
		return nil
	}
}

func isLoopbackAddress(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
