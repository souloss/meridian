package command

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/meridian-labs/meridian/internal/handler"
	"github.com/spf13/cobra"
)

func newServeCommand() *cobra.Command {
	var addr string

	command := &cobra.Command{
		Use:   "serve",
		Short: "Start the Meridian HTTP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return runServer(command.Context(), addr, command.ErrOrStderr())
		},
	}
	command.Flags().StringVar(&addr, "addr", ":8080", "HTTP listen address")
	return command
}

func runServer(ctx context.Context, addr string, output io.Writer) error {
	logger := slog.New(slog.NewJSONHandler(output, nil))
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
