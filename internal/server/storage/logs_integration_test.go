//go:build integration

package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kamerrezz/theminidog/internal/model"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func TestLogRepository_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	repo := storage.NewLogRepository(pool)

	t.Run("ping", func(t *testing.T) {
		if err := repo.Ping(ctx); err != nil {
			t.Fatalf("ping: %v", err)
		}
	})

	t.Run("insert and query", func(t *testing.T) {
		batch := model.LogBatch{
			Host: "test-host",
			Entries: []model.LogEntry{
				{Time: time.Now().UTC(), Host: "test-host", Path: "/var/log/app.log", Level: model.LevelError, Message: "connection timeout"},
				{Time: time.Now().UTC(), Host: "test-host", Path: "/var/log/app.log", Level: model.LevelInfo, Message: "server started"},
			},
		}
		n, err := repo.Insert(ctx, batch)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}
		if n != 2 {
			t.Fatalf("expected 2 inserted, got %d", n)
		}

		params := storage.LogQueryParams{
			Host:  "test-host",
			Limit: 10,
		}
		results, _, err := repo.Query(ctx, params)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(results) < 2 {
			t.Fatalf("expected at least 2 results, got %d", len(results))
		}
	})
}
