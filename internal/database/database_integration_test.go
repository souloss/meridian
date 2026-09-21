//go:build integration

package database

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"
)

var expectedM0Tables = []string{
	"ai_generation_results",
	"api_tokens",
	"asset_items",
	"asset_kinds",
	"asset_ref_tracks",
	"asset_versions",
	"assets",
	"audit_logs",
	"blobs",
	"breaking_todos",
	"config_import_previews",
	"credential_team_shares",
	"credentials",
	"discovery_candidates",
	"diff_rule_sets",
	"diff_snapshots",
	"global_credentials",
	"global_idempotency_records",
	"idempotency_records",
	"job_stage_logs",
	"jobs",
	"known_hosts",
	"layer_heads",
	"layer_revisions",
	"layers",
	"notification_channels",
	"notify_outbox",
	"platform_settings",
	"producer_profiles",
	"recent_services",
	"refresh_tokens",
	"repositories",
	"services",
	"share_links",
	"source_bindings",
	"source_specs",
	"system_group_members",
	"system_groups",
	"team_members",
	"teams",
	"tenant_blob_refs",
	"tenant_kind_overrides",
	"tenant_members",
	"tenants",
	"uploads",
	"user_preferences",
	"users",
}

const expectedApplicationMigrationVersion = 11

func TestMigrationLifecycle(t *testing.T) {
	databaseURL := os.Getenv("MERIDIAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MERIDIAN_TEST_DATABASE_URL is not set")
	}

	db, err := Open(t.Context(), databaseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})

	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("initial migration up: %v", err)
	}
	first := schemaSnapshot(t, db.SQL)
	assertM0Schema(t, db)

	if err := db.MigrateDown(t.Context(), 1); err != nil {
		t.Fatalf("latest migration down: %v", err)
	}
	status, err := db.MigrationStatus(t.Context())
	if err != nil {
		t.Fatalf("migration status after latest down: %v", err)
	}
	if status.ApplicationVersion != expectedApplicationMigrationVersion-1 || len(status.RiverVersions) == 0 {
		t.Fatalf("migration status after latest down = %#v, want application %d with River versions", status, expectedApplicationMigrationVersion-1)
	}
	if err := db.MigrateDown(t.Context(), expectedApplicationMigrationVersion-1); err != nil {
		t.Fatalf("baseline migration down: %v", err)
	}
	assertSchemaRemoved(t, db.SQL)
	status, err = db.MigrationStatus(t.Context())
	if err != nil {
		t.Fatalf("migration status after down: %v", err)
	}
	if status.ApplicationVersion != 0 || len(status.RiverVersions) != 0 {
		t.Fatalf("migration status after down = %#v, want zero versions", status)
	}

	if err := db.MigrateUp(t.Context()); err != nil {
		t.Fatalf("second migration up: %v", err)
	}
	second := schemaSnapshot(t, db.SQL)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("schema differs after up/down/up\nfirst:  %v\nsecond: %v", first, second)
	}
	assertM0Schema(t, db)
}

func TestConcurrentMigrationUp(t *testing.T) {
	databaseURL := os.Getenv("MERIDIAN_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("MERIDIAN_TEST_DATABASE_URL is not set")
	}

	const instanceCount = 2
	databases := make([]*Database, 0, instanceCount)
	for range instanceCount {
		db, err := Open(t.Context(), databaseURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatalf("open database: %v", err)
		}
		databases = append(databases, db)
		t.Cleanup(func() {
			if err := db.Close(); err != nil {
				t.Errorf("close database: %v", err)
			}
		})
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	errorsByInstance := make([]error, instanceCount)
	var waitGroup sync.WaitGroup
	for index, db := range databases {
		waitGroup.Go(func() {
			errorsByInstance[index] = db.MigrateUp(ctx)
		})
	}
	waitGroup.Wait()
	for index, err := range errorsByInstance {
		if err != nil {
			t.Fatalf("concurrent migration instance %d: %v", index, err)
		}
	}
	assertM0Schema(t, databases[0])
}

func assertM0Schema(t *testing.T, database *Database) {
	t.Helper()
	db := database.SQL

	if got := publicTables(t, db); !reflect.DeepEqual(got, expectedM0Tables) {
		t.Fatalf("public tables = %v, want %v", got, expectedM0Tables)
	}

	var settingsCount int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM platform_settings WHERE id = 'default'`).Scan(&settingsCount); err != nil {
		t.Fatalf("read platform settings seed: %v", err)
	}
	if settingsCount != 1 {
		t.Fatalf("platform settings seed count = %d, want 1", settingsCount)
	}

	var missingTableComments int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname <> 'goose_db_version'
		  AND nullif(obj_description(c.oid, 'pg_class'), '') IS NULL
	`).Scan(&missingTableComments); err != nil {
		t.Fatalf("count missing table comments: %v", err)
	}
	if missingTableComments != 0 {
		t.Fatalf("tables without comments = %d", missingTableComments)
	}

	var missingColumnComments int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname <> 'goose_db_version'
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		  AND nullif(col_description(c.oid, a.attnum), '') IS NULL
	`).Scan(&missingColumnComments); err != nil {
		t.Fatalf("count missing column comments: %v", err)
	}
	if missingColumnComments != 0 {
		t.Fatalf("columns without comments = %d", missingColumnComments)
	}

	var riverTableCount int
	if err := db.QueryRowContext(t.Context(), `
		SELECT count(*)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'river' AND c.relkind = 'r'
	`).Scan(&riverTableCount); err != nil {
		t.Fatalf("count River tables: %v", err)
	}
	if riverTableCount == 0 {
		t.Fatal("River schema has no tables")
	}

	status, err := database.MigrationStatus(t.Context())
	if err != nil {
		t.Fatalf("read migration status: %v", err)
	}
	if status.ApplicationVersion != expectedApplicationMigrationVersion || len(status.RiverVersions) == 0 {
		t.Fatalf("migration status = %#v, want application version %d and River versions", status, expectedApplicationMigrationVersion)
	}
}

func assertSchemaRemoved(t *testing.T, db *sql.DB) {
	t.Helper()

	if tables := publicTables(t, db); len(tables) != 0 {
		t.Fatalf("public tables remain after down: %v", tables)
	}
	var riverSchemaExists bool
	if err := db.QueryRowContext(t.Context(), `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'river')`).Scan(&riverSchemaExists); err != nil {
		t.Fatalf("check River schema: %v", err)
	}
	if riverSchemaExists {
		t.Fatal("River schema remains after full application rollback")
	}
}

func publicTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname <> 'goose_db_version'
		ORDER BY c.relname
	`)
	if err != nil {
		t.Fatalf("list public tables: %v", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	return tables
}

func schemaSnapshot(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `
		SELECT c.relname || '.' || a.attname || ':' || format_type(a.atttypid, a.atttypmod) || ':' || a.attnotnull::text || ':' || coalesce(col_description(c.oid, a.attnum), '')
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE n.nspname = 'public'
		  AND c.relkind = 'r'
		  AND c.relname <> 'goose_db_version'
		  AND a.attnum > 0
		  AND NOT a.attisdropped
		ORDER BY c.relname, a.attnum
	`)
	if err != nil {
		t.Fatalf("snapshot schema: %v", err)
	}
	defer rows.Close()

	var snapshot []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan schema snapshot: %v", err)
		}
		snapshot = append(snapshot, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema snapshot: %v", err)
	}
	return slices.Clip(snapshot)
}
