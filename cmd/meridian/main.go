package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/meridian-labs/meridian/internal/command"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := command.Execute(ctx, os.Stdout, os.Stderr); err != nil {
		var cliErr *command.CliError
		if errors.As(err, &cliErr) {
			_, _ = fmt.Fprintln(os.Stderr, cliErr)
			os.Exit(cliErr.ExitCode())
		}
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
