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

	return ServerConfig{
		ListenAddr:      listenAddr,
		DatabaseURL:     dbURL,
		AgentToken:      token,
		MigrationsPath:  migrationsPath,
		LogLevel:        logLevel,
		RequestTimeout:  reqTimeout,
		ShutdownTimeout: shutdownTimeout,
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
