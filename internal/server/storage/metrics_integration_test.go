//go:build integration

package storage_test

import (
	"os"
	"testing"
)

func TestMetricRepository_Integration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	// TODO: Week 2 integration tests using real TimescaleDB
}
