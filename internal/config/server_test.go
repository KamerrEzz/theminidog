package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadServerConfig_AllRequiredPresent_Success(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@host:5432/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://user:pass@host:5432/db" {
		t.Errorf("DatabaseURL mismatch: got %q", cfg.DatabaseURL)
	}
	if cfg.AgentToken != "my-secret-token-abc" {
		t.Errorf("AgentToken mismatch: got %q", cfg.AgentToken)
	}
}

func TestLoadServerConfig_MissingDatabaseURL_ReturnsError(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")

	_, err := LoadServerConfig()
	if err == nil {
		t.Fatal("expected error when DATABASE_URL is missing, got nil")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error should mention DATABASE_URL, got: %q", err.Error())
	}
}

func TestLoadServerConfig_InvalidDatabaseURLScheme_ReturnsError(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://user:pass@host/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")

	_, err := LoadServerConfig()
	if err == nil {
		t.Fatal("expected error for mysql:// scheme, got nil")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("error should mention postgres, got: %q", err.Error())
	}
}

func TestLoadServerConfig_PostgresqlScheme_Success(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://user:pass@host:5432/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")

	_, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("expected success for postgresql:// scheme, got: %v", err)
	}
}

func TestLoadServerConfig_MissingAgentToken_ReturnsError(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "")

	_, err := LoadServerConfig()
	if err == nil {
		t.Fatal("expected error when AGENT_TOKEN is missing, got nil")
	}
	if !strings.Contains(err.Error(), "AGENT_TOKEN") {
		t.Errorf("error should mention AGENT_TOKEN, got: %q", err.Error())
	}
}

func TestLoadServerConfig_AgentTokenTooShort_ReturnsError(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "tooshort") // 8 characters

	_, err := LoadServerConfig()
	if err == nil {
		t.Fatal("expected error for short AGENT_TOKEN, got nil")
	}
	if !strings.Contains(err.Error(), "16") {
		t.Errorf("error should mention minimum length 16, got: %q", err.Error())
	}
}

func TestLoadServerConfig_AgentToken16Chars_Success(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "exactly16charssss") // 17 chars, >= 16

	_, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("expected success for 16+ char token, got: %v", err)
	}
}

func TestLoadServerConfig_InvalidLogLevel_DefaultsToInfo(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")
	t.Setenv("LOG_LEVEL", "verbose")

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=info for invalid value, got: %q", cfg.LogLevel)
	}
}

func TestLoadServerConfig_LogLevelDebug_Accepted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel=debug, got: %q", cfg.LogLevel)
	}
}

func TestLoadServerConfig_RequestTimeout_OutOfRange_FallsBack(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")
	t.Setenv("REQUEST_TIMEOUT", "200s") // exceeds max 120s

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("expected fallback to 10s for out-of-range timeout, got: %v", cfg.RequestTimeout)
	}
}

func TestLoadServerConfig_RequestTimeout_ValidValue_Accepted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")
	t.Setenv("REQUEST_TIMEOUT", "30s")

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RequestTimeout != 30*time.Second {
		t.Errorf("expected RequestTimeout=30s, got: %v", cfg.RequestTimeout)
	}
}

func TestLoadServerConfig_AllOptionalAbsent_CorrectDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/db")
	t.Setenv("AGENT_TOKEN", "my-secret-token-abc")
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("MIGRATIONS_PATH", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("REQUEST_TIMEOUT", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")

	cfg, err := LoadServerConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Errorf("expected ListenAddr=:8080, got: %q", cfg.ListenAddr)
	}
	if cfg.MigrationsPath != "./migrations" {
		t.Errorf("expected MigrationsPath=./migrations, got: %q", cfg.MigrationsPath)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=info, got: %q", cfg.LogLevel)
	}
	if cfg.RequestTimeout != 10*time.Second {
		t.Errorf("expected RequestTimeout=10s, got: %v", cfg.RequestTimeout)
	}
	if cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected ShutdownTimeout=5s, got: %v", cfg.ShutdownTimeout)
	}
}
