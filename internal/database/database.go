// Package database owns PostgreSQL connectivity and schema lifecycle.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/meridian-labs/meridian/migrations"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverdatabasesql"
	"github.com/riverqueue/river/rivermigrate"
)

const migrationLockName = "meridian:schema-migrations"
const riverSchema = "river"

// Database owns a connected PostgreSQL pool and its migration lifecycle.
type Database struct {
	SQL    *sql.DB
	logger *slog.Logger
	mu     sync.Mutex
}

// Open connects to PostgreSQL and verifies that the server is reachable.
func Open(ctx context.Context, databaseURL string, logger *slog.Logger) (*Database, error) {
	if databaseURL == "" {
		return nil, errors.New("MERIDIAN_DATABASE_URL is required")
	}
	if logger == nil {
		logger = slog.Default()
	}

	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Database{SQL: db, logger: logger}, nil
}

// Close releases every PostgreSQL connection owned by Database.
func (db *Database) Close() error {
	if db == nil || db.SQL == nil {
		return nil
	}
	return db.SQL.Close()
}

// MigrateUp upgrades River and then the Meridian schema to their pinned targets.
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

// MigrateDown rolls back explicit application steps and removes River when the
// application schema reaches version zero. Startup never calls this method.
func (db *Database) MigrateDown(ctx context.Context, steps int) error {
	if steps < 1 {
		return errors.New("migration down steps must be greater than zero")
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
		if applicationVersion == 0 {
			if _, err := db.migrateRiver(ctx, rivermigrate.DirectionDown, &rivermigrate.MigrateOpts{TargetVersion: -1}); err != nil {
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

// Status is the current application schema version and applied River versions.
type Status struct {
	// ApplicationVersion is the latest applied goose migration version.
	ApplicationVersion int64
	// RiverVersions contains River's applied migrations in ascending order.
	RiverVersions []rivermigrate.Migration
}

// MigrationStatus reads the current application and River migration versions.
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
	river, err := rivermigrate.New(riverdatabasesql.New(db.SQL), &rivermigrate.Config{Schema: riverSchema})
	if err != nil {
		return Status{}, fmt.Errorf("create River migrator: %w", err)
	}
	riverVersions, err := river.ExistingVersions(ctx)
	if err != nil {
		return Status{}, fmt.Errorf("read River migration versions: %w", err)
	}
	return Status{ApplicationVersion: applicationVersion, RiverVersions: riverVersions}, nil
}

func (db *Database) migrateRiver(ctx context.Context, direction rivermigrate.Direction, opts *rivermigrate.MigrateOpts) (*rivermigrate.MigrateResult, error) {
	migrator, err := rivermigrate.New(riverdatabasesql.New(db.SQL), &rivermigrate.Config{Schema: riverSchema})
	if err != nil {
		return nil, err
	}
	return migrator.Migrate(ctx, direction, opts)
}

func (db *Database) withMigrationLock(ctx context.Context, operation func(context.Context) error) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	// A dedicated connection owns the session-level lock while River and goose
	// use the pool. Other Meridian processes must acquire the same lock before
	// they can mutate either schema.
	lockConnection, err := db.SQL.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migration lock connection: %w", err)
	}
	defer lockConnection.Close()

	if _, err := lockConnection.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, migrationLockName); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, err := lockConnection.ExecContext(unlockContext, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, migrationLockName); err != nil {
			db.logger.Error("release migration lock failed", "error", err)
		}
	}()
	return operation(ctx)
}
