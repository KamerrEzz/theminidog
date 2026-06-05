package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kamerrezz/theminidog/internal/config"
	"github.com/kamerrezz/theminidog/internal/server"
	"github.com/kamerrezz/theminidog/internal/server/alerting"
	"github.com/kamerrezz/theminidog/internal/server/api"
	"github.com/kamerrezz/theminidog/internal/server/dashboard"
	"github.com/kamerrezz/theminidog/internal/server/storage"
)

func main() {
	cfg, err := config.LoadServerConfig()
	if err != nil {
		slog.Error("invalid configuration", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	// Run migrations using pgx5:// scheme required by golang-migrate pgx/v5 driver.
	// The postgres:// DSN is used for pgxpool (separate path).
	migrateURL := strings.Replace(cfg.DatabaseURL, "postgres://", "pgx5://", 1)
	migrateURL = strings.Replace(migrateURL, "postgresql://", "pgx5://", 1)
	if err := runMigrations(fmt.Sprintf("file://%s", cfg.MigrationsPath), migrateURL); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to create connection pool", "err", err)
		os.Exit(1)
	}

	repo := storage.NewMetricRepository(pool)
	logRepo := storage.NewLogRepository(pool)

	// Parse alert notifications (optional; empty = disabled).
	notifiers, err := alerting.ParseNotifications(cfg.AlertNotifications)
	if err != nil {
		slog.Error("invalid ALERT_NOTIFICATIONS", "err", err)
		os.Exit(1)
	}

	// Parse alert rules and conditionally create the evaluator.
	var evaluator *alerting.Evaluator
	if cfg.AlertRules != "" {
		rules, err := alerting.ParseRules(cfg.AlertRules)
		if err != nil {
			slog.Error("invalid ALERT_RULES", "err", err)
			os.Exit(1)
		}
		if len(rules) > 0 {
			evaluator = alerting.NewEvaluator(rules, repo, log, alerting.WithNotifiers(notifiers))
		}
	}

	// Build the dashboard handler when enabled.
	var dash *dashboard.DashHandler
	if cfg.DashboardEnabled {
		dash = dashboard.NewDashHandler(repo, logRepo, evaluator)
	}

	// Build a clean nil interface for alerter (ADR-3: avoid typed-nil pitfall).
	// The router's alerter != nil guard must see a true nil interface, not a typed nil.
	var alerter alerting.AlertReader
	if evaluator != nil {
		alerter = evaluator
	}

	router := api.NewRouter(repo, logRepo, []byte(cfg.AgentToken), cfg.RequestTimeout, dash, alerter)
	srv := server.New(cfg.ListenAddr, router, pool, log)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start the alert evaluator AFTER the signal context is created (ADR-6).
	if evaluator != nil {
		go evaluator.Run(ctx)
	}

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
	slog.Info("server shutdown complete")
}

func runMigrations(sourceURL, databaseURL string) error {
	m, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
