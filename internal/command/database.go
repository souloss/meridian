package command

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/meridian-labs/meridian/internal/database"
)

func openDatabase(ctx context.Context, databaseURL string, output io.Writer) (*database.Database, error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	return database.Open(ctx, databaseURL, logger)
}

func closeDatabase(db *database.Database, currentErr *error) {
	*currentErr = errors.Join(*currentErr, db.Close())
}
