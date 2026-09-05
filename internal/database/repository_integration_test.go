//go:build integration

package database

import (
	"io"
	"log/slog"
	"os"
	"testing"
	"uuid"

	generated "github.com/meridian-labs/meridian/internal/generated/repository"
)

func TestGeneratedRepositoryUsesStandardUUIDAndTransaction(t *testing.T) {
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
		t.Fatalf("migrate database: %v", err)
	}

	tx, err := db.Pool.Begin(t.Context())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() {
		_ = tx.Rollback(t.Context())
	})

	queries := generated.New(tx)
	userID := uuid.NewV7()
	created, err := queries.CreateUser(t.Context(), generated.CreateUserParams{
		ID:              userID,
		Username:        "repository-transaction-test",
		PasswordHash:    "$argon2id$v=19$m=65536,t=3,p=1$fixture$fixture",
		DisplayName:     "Repository transaction test",
		IsPlatformAdmin: false,
	})
	if err != nil {
		t.Fatalf("create user with standard UUID: %v", err)
	}
	if created.ID != userID {
		t.Fatalf("created user ID = %s, want %s", created.ID, userID)
	}
	if _, err := queries.CreateDefaultUserPreferences(t.Context(), userID); err != nil {
		t.Fatalf("create user preferences: %v", err)
	}
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatalf("roll back transaction: %v", err)
	}

	var count int
	if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count rolled-back user: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back user count = %d, want 0", count)
	}
}
