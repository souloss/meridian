package command

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/meridian-labs/meridian/internal/database"
)

// openDatabase 使用 JSON 日志与给定连接串打开数据库连接。
func openDatabase(ctx context.Context, databaseURL string, output io.Writer) (*database.Database, error) {
	logger := slog.New(slog.NewJSONHandler(output, nil))
	return database.Open(ctx, databaseURL, logger)
}

// closeDatabase 将数据库关闭错误并入当前错误链。
func closeDatabase(db *database.Database, currentErr *error) {
	*currentErr = errors.Join(*currentErr, db.Close())
}
