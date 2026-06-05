package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// ServerConfig holds all runtime configuration for the metrics server.
type ServerConfig struct {
	ListenAddr      string
	DatabaseURL     string
	AgentToken      string
	MigrationsPath  string
	LogLevel        string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	AlertRules           string // raw JSON from ALERT_RULES; "" = alerting disabled
	AlertNotifications   string // raw JSON from ALERT_NOTIFICATIONS; "" = notifications disabled
	DashboardEnabled     bool   // from DASHBOARD_ENABLED; default true
	HostStaleAfter  time.Duration // from HOST_STALE_AFTER; default 20s
	HostDownAfter   time.Duration // from HOST_DOWN_AFTER; default 50s
}

// LoadServerConfig reads ServerConfig from environment variables.
// Returns an error (fail-fast) for any missing required variable or invalid value.
func LoadServerConfig() (ServerConfig, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return ServerConfig{}, fmt.Errorf("DATABASE_URL is required but not set")
	}
	u, err := url.Parse(dbURL)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return ServerConfig{}, fmt.Errorf("DATABASE_URL must be a valid postgres:// URL, got %q", dbURL)
	}

	token := os.Getenv("AGENT_TOKEN")
	if token == "" {
		return ServerConfig{}, fmt.Errorf("AGENT_TOKEN is required but not set")
	}
	if len(token) < 16 {
		return ServerConfig{}, fmt.Errorf("AGENT_TOKEN must be at least 16 characters")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":8080"
	}

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "./migrations"
	}

	logLevel := "info"
	if ll := strings.ToLower(os.Getenv("LOG_LEVEL")); ll == "debug" || ll == "warn" || ll == "error" {
		logLevel = ll
	}

	reqTimeout := parseDuration(os.Getenv("REQUEST_TIMEOUT"), 10*time.Second, time.Second, 120*time.Second)
	shutdownTimeout := parseDuration(os.Getenv("SHUTDOWN_TIMEOUT"), 5*time.Second, time.Second, 30*time.Second)

	// ALERT_RULES: raw JSON string; empty string means alerting is disabled.
	alertRules := os.Getenv("ALERT_RULES")

	// ALERT_NOTIFICATIONS: raw JSON array; empty string means notifications disabled.
	alertNotifications := os.Getenv("ALERT_NOTIFICATIONS")

	// DASHBOARD_ENABLED: default true; explicitly disabled when value is "false" or "0".
	dashEnabled := true
	if v := strings.ToLower(os.Getenv("DASHBOARD_ENABLED")); v == "false" || v == "0" {
		dashEnabled = false
	}

	// HOST_STALE_AFTER / HOST_DOWN_AFTER: fail-fast on unparseable values.
	hostStaleAfter, err := parseDurationStrict(os.Getenv("HOST_STALE_AFTER"), 20*time.Second)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("HOST_STALE_AFTER: %w", err)
	}
	hostDownAfter, err := parseDurationStrict(os.Getenv("HOST_DOWN_AFTER"), 50*time.Second)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("HOST_DOWN_AFTER: %w", err)
	}

	return ServerConfig{
		ListenAddr:       listenAddr,
		DatabaseURL:      dbURL,
		AgentToken:       token,
		MigrationsPath:   migrationsPath,
		LogLevel:         logLevel,
		RequestTimeout:   reqTimeout,
		ShutdownTimeout:  shutdownTimeout,
		AlertRules:         alertRules,
		AlertNotifications: alertNotifications,
		DashboardEnabled:   dashEnabled,
		HostStaleAfter:     hostStaleAfter,
		HostDownAfter:      hostDownAfter,
	}, nil
}

// parseDuration parses a duration string with min/max clamping.
// Returns def on empty input, parse error, or out-of-range value.
func parseDuration(raw string, def, min, max time.Duration) time.Duration {
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < min || d > max {
		return def
	}
	return d
}

// parseDurationStrict parses a duration string, returning the default on empty input
// and a descriptive error on any non-empty unparseable value (fail-fast).
func parseDurationStrict(raw string, def time.Duration) (time.Duration, error) {
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %q", raw)
	}
	return d, nil
}
