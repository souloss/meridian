package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/meridian-labs/meridian/internal/handler"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var addr string
	var databaseURL string

	command := &cobra.Command{
		Use:   "serve",
		Short: "Start the Meridian HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runServer(command.Context(), addr, databaseURL, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	command.Flags().StringVar(&databaseURL, "database-url", os.Getenv("MERIDIAN_DATABASE_URL"), "PostgreSQL connection URL (or MERIDIAN_DATABASE_URL)")
	return command
}

func runServer(ctx context.Context, addr, databaseURL string, output io.Writer) (err error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	db, err := openDatabase(ctx, databaseURL, output)
	if err != nil {
		return err
	}
	defer closeDatabase(db, &err)
	if err := db.MigrateUp(ctx); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           handler.New().Handler(),
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
