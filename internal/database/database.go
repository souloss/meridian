// Package database 拥有 PostgreSQL 连接与 schema 生命周期。
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/meridian-labs/meridian/migrations"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

// 数据库连接池与迁移常量。
const (
	// migrationLockName 是跨进程迁移互斥使用的 advisory lock 名。
	migrationLockName = "meridian:schema-migrations"
	// riverSchema 是 River 队列使用的数据库 schema 名。
	riverSchema = "river"
	// poolMaxConnections 是 pgx 连接池允许的最大连接数。
	poolMaxConnections = 16
	// poolMinIdleConnections 是 pgx 连接池保持的最小空闲连接数。
	poolMinIdleConnections = 4
	// schemaVersionZero 是应用 schema 回滚到底的版本号。
	schemaVersionZero = 0
	// migrationLockReleaseTimeout 是释放迁移锁的宽限时长。
	migrationLockReleaseTimeout = 5 * time.Second
)

// 数据库包内哨兵错误。
var (
	// errDatabaseURLRequired 表示未提供数据库连接串环境变量。
	errDatabaseURLRequired = errors.New("MERIDIAN_DATABASE_URL is required")
	// errMigrationStepsNonPositive 表示回滚步数必须为正整数。
	errMigrationStepsNonPositive = errors.New("migration down steps must be greater than zero")
)

// riverTargetVersionZero 是 River 全部回滚的目标版本。
const riverTargetVersionZero = -1

// Database 拥有一个已连接的 PostgreSQL 连接池及其迁移生命周期。
type Database struct {
	// Pool 是 sqlc 查询与 River 事务使用的原生 pgx 连接池。
	Pool *pgxpool.Pool
	// SQL 是架设在 Pool 之上的 database/sql 适配器，仅供 goose 与基于 SQL 的诊断使用。
	// 关闭它不会关闭 Pool。
	SQL    *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// Open 连接 PostgreSQL 并校验服务器可达。
func Open(ctx context.Context, databaseURL string, logger *slog.Logger) (*Database, error) {
	if databaseURL == "" {
		return nil, errDatabaseURLRequired
	}
	if logger == nil {
		logger = slog.Default()
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	poolConfig.MaxConns = poolMaxConnections
	poolConfig.MinIdleConns = poolMinIdleConnections
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Database{
		Pool:   pool,
		SQL:    stdlib.OpenDBFromPool(pool),
		logger: logger,
	}, nil
}

// Close 释放 Database 拥有的每个 PostgreSQL 连接。
func (db *Database) Close() (err error) {
	if db == nil {
		return nil
	}
	if db.SQL != nil {
		err = db.SQL.Close()
	}
	if db.Pool != nil {
		db.Pool.Close()
	}
	return err
}

// MigrateUp 先升级 River 再升级 Meridian schema 到各自固定目标版本。
func (db *Database) MigrateUp(ctx context.Context) error {
	return db.withMigrationLock(ctx, func(ctx context.Context) error {
		if _, err := db.SQL.ExecContext(ctx, `CREATE SCHEMA IF NOT EXISTS river`); err != nil {
			return fmt.Errorf("create River schema: %w", err)
		}
		if _, err := db.SQL.ExecContext(ctx, `COMMENT ON SCHEMA river IS 'Schema owned and migrated by River v0.33.0.'`); err != nil {
			return fmt.Errorf("comment River schema: %w", err)
		}
		if _, err := db.migrateRiver(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return fmt.Errorf("migrate River up: %w", err)
		}
		provider, err := goose.NewProvider(goose.DialectPostgres, db.SQL, migrations.FS)
		if err != nil {
			return fmt.Errorf("create goose provider: %w", err)
		}
		if _, err := provider.Up(ctx); err != nil {
			return fmt.Errorf("migrate application schema up: %w", err)
		}
		db.logger.InfoContext(ctx, "database migrations applied")
		return nil
	})
}

// MigrateDown 回滚显式数量的应用迁移步骤，并在应用 schema 归零时移除 River。
// 启动过程从不调用此方法。
func (db *Database) MigrateDown(ctx context.Context, steps int) error {
	if steps < 1 {
		return errMigrationStepsNonPositive
	}
	return db.withMigrationLock(ctx, func(ctx context.Context) error {
		provider, err := goose.NewProvider(goose.DialectPostgres, db.SQL, migrations.FS)
		if err != nil {
			return fmt.Errorf("create goose provider: %w", err)
		}
		for range steps {
			if _, err := provider.Down(ctx); err != nil {
				return fmt.Errorf("migrate application schema down: %w", err)
			}
		}
		applicationVersion, err := provider.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("read application migration version: %w", err)
		}
		if applicationVersion == schemaVersionZero {
			if _, err := db.migrateRiver(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{TargetVersion: riverTargetVersionZero}); err != nil {
				return fmt.Errorf("migrate River down: %w", err)
			}
			if _, err := db.SQL.ExecContext(ctx, `DROP SCHEMA IF EXISTS river`); err != nil {
				return fmt.Errorf("drop River schema: %w", err)
			}
		}
		db.logger.InfoContext(ctx, "database migrations rolled back", "steps", steps)
		return nil
	})
}

// Status 是当前应用 schema 版本与已应用的 River 版本。
type Status struct {
	// ApplicationVersion 是最近应用的 goose 迁移版本。
	ApplicationVersion int64
	// RiverVersions 按升序包含 River 已应用的迁移。
	RiverVersions []rivermigrate.Migration
}

// MigrationStatus 读取当前应用与 River 迁移版本。
func (db *Database) MigrationStatus(ctx context.Context) (Status, error) {
	provider, err := goose.NewProvider(goose.DialectPostgres, db.SQL, migrations.FS)
	if err != nil {
		return Status{}, fmt.Errorf("create goose provider: %w", err)
	}
	applicationVersion, err := provider.GetDBVersion(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read application migration version: %w", err)
	}
	var riverInitialized bool
	if err := db.SQL.QueryRowContext(ctx, `SELECT to_regclass('river.river_migration') IS NOT NULL`).Scan(&riverInitialized); err != nil {
		return Status{}, fmt.Errorf("check River migration table: %w", err)
	}
	if !riverInitialized {
		return Status{ApplicationVersion: applicationVersion}, nil
	}
	river, err := rivermigrate.New(riverpgxv5.New(db.Pool), &rivermigrate.Config{Schema: riverSchema})
	if err != nil {
		return Status{}, fmt.Errorf("create River migrator: %w", err)
	}
	riverVersions, err := river.ExistingVersions(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read River migration versions: %w", err)
	}
	return Status{ApplicationVersion: applicationVersion, RiverVersions: riverVersions}, nil
}

// migrateRiver 使用共享的 River migrator 配置执行指定方向的迁移。
func (db *Database) migrateRiver(ctx context.Context, direction rivermigrate.Direction, opts *rivermigrate.MigrateOpts) (*rivermigrate.MigrateResult, error) {
	migrator, err := rivermigrate.New(riverpgxv5.New(db.Pool), &rivermigrate.Config{Schema: riverSchema})
	if err != nil {
		return nil, err
	}
	return migrator.Migrate(ctx, direction, opts)
}

// withMigrationLock 在会话级 advisory lock 保护下执行一次 schema 迁移操作。
func (db *Database) withMigrationLock(ctx context.Context, operation func(context.Context) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// 一个专用连接持有会话级锁，而 River 与 goose 使用连接池。
	// 其他 Meridian 进程在改动任一 schema 之前必须先取得同一把锁。
	lockConnection, err := db.Pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration lock connection: %w", err)
	}
	defer lockConnection.Release()

	if _, err := lockConnection.Exec(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, migrationLockName); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), migrationLockReleaseTimeout)
		defer cancel()
		if _, err := lockConnection.Exec(unlockContext, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, migrationLockName); err != nil {
			db.logger.Error("release migration lock failed", "error", err)
		}
	}()
	return operation(ctx)
}
