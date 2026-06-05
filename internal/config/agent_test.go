package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ── Missing / invalid SERVER_URL ─────────────────────────────────────────────

func TestLoadAgent_MissingServerURL_ReturnsError(t *testing.T) {
	t.Setenv("SERVER_URL", "")
	_, err := LoadAgent()
	if err == nil {
		t.Fatal("expected error when SERVER_URL is missing, got nil")
	}
	if !strings.Contains(err.Error(), "SERVER_URL") {
		t.Errorf("error should mention SERVER_URL, got: %q", err.Error())
	}
}

func TestLoadAgent_InvalidScheme_FTP_ReturnsError(t *testing.T) {
	t.Setenv("SERVER_URL", "ftp://example.com")
	_, err := LoadAgent()
	if err == nil {
		t.Fatal("expected error for ftp:// scheme, got nil")
	}
}

func TestLoadAgent_InvalidScheme_Empty_ReturnsError(t *testing.T) {
	t.Setenv("SERVER_URL", "not-a-url")
	_, err := LoadAgent()
	if err == nil {
		t.Fatal("expected error for non-URL value, got nil")
	}
}

// ── Valid SERVER_URL ─────────────────────────────────────────────────────────

func TestLoadAgent_ValidHTTPURL_Success(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("expected success for valid http URL, got: %v", err)
	}
	if cfg.ServerURL == nil {
		t.Fatal("ServerURL must not be nil")
	}
	if cfg.ServerURL.String() != "http://localhost:8080" {
		t.Errorf("ServerURL mismatch: got %q, want %q", cfg.ServerURL.String(), "http://localhost:8080")
	}
}

func TestLoadAgent_ValidHTTPSURL_Success(t *testing.T) {
	t.Setenv("SERVER_URL", "https://collector.example.com:9090")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("expected success for valid https URL, got: %v", err)
	}
	if cfg.ServerURL.Scheme != "https" {
		t.Errorf("expected scheme https, got %q", cfg.ServerURL.Scheme)
	}
}

// ── COLLECT_INTERVAL ─────────────────────────────────────────────────────────

func TestLoadAgent_CollectInterval_OutOfRange_High_FallsBack(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("COLLECT_INTERVAL", "500s")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CollectInterval != 10*time.Second {
		t.Errorf("expected fallback to 10s for 500s interval, got: %v", cfg.CollectInterval)
	}
}

func TestLoadAgent_CollectInterval_Invalid_String_FallsBack(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("COLLECT_INTERVAL", "abc")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CollectInterval != 10*time.Second {
		t.Errorf("expected fallback to 10s for invalid interval, got: %v", cfg.CollectInterval)
	}
}

func TestLoadAgent_CollectInterval_ValidValue_Accepted(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("COLLECT_INTERVAL", "5s")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CollectInterval != 5*time.Second {
		t.Errorf("expected 5s interval, got: %v", cfg.CollectInterval)
	}
}

func TestLoadAgent_CollectInterval_LowerBound_OneSecond_Accepted(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("COLLECT_INTERVAL", "1s")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CollectInterval != time.Second {
		t.Errorf("expected 1s interval, got: %v", cfg.CollectInterval)
	}
}

func TestLoadAgent_CollectInterval_TooLow_FallsBack(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("COLLECT_INTERVAL", "500ms")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CollectInterval != 10*time.Second {
		t.Errorf("expected fallback to 10s for 500ms interval, got: %v", cfg.CollectInterval)
	}
}

// ── LOG_LEVEL ────────────────────────────────────────────────────────────────

func TestLoadAgent_LogLevel_Invalid_FallsBackToInfo(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_LEVEL", "verbose")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel=info for invalid value, got: %q", cfg.LogLevel)
	}
}

func TestLoadAgent_LogLevel_Warn_Accepted(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_LEVEL", "warn")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("expected LogLevel=warn, got: %q", cfg.LogLevel)
	}
}

func TestLoadAgent_LogLevel_Debug_Accepted(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("expected LogLevel=debug, got: %q", cfg.LogLevel)
	}
}

func TestLoadAgent_LogLevel_Error_Accepted(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_LEVEL", "error")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("expected LogLevel=error, got: %q", cfg.LogLevel)
	}
}

// ── AGENT_HOST ───────────────────────────────────────────────────────────────

func TestLoadAgent_AgentHost_ExplicitValue_Used(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("AGENT_HOST", "myhost")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AgentHost != "myhost" {
		t.Errorf("expected AgentHost=myhost, got: %q", cfg.AgentHost)
	}
}

func TestLoadAgent_AgentHost_Unset_FallsBackToOSHostname(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("AGENT_HOST", "")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected, _ := os.Hostname()
	if expected != "" && cfg.AgentHost != expected {
		t.Errorf("expected AgentHost=%q (os.Hostname), got: %q", expected, cfg.AgentHost)
	}
	if cfg.AgentHost == "" {
		t.Error("AgentHost must not be empty when os.Hostname succeeds")
	}
}

// ── LOG_PATHS ────────────────────────────────────────────────────────────────

func TestLoadAgent_LogPaths_TwoEntries(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_PATHS", "/var/log/app.log,/var/log/err.log")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.LogPaths) != 2 {
		t.Fatalf("expected 2 log paths, got %d: %v", len(cfg.LogPaths), cfg.LogPaths)
	}
	if cfg.LogPaths[0] != "/var/log/app.log" {
		t.Errorf("first path mismatch: got %q", cfg.LogPaths[0])
	}
	if cfg.LogPaths[1] != "/var/log/err.log" {
		t.Errorf("second path mismatch: got %q", cfg.LogPaths[1])
	}
}

func TestLoadAgent_LogPaths_EmptyString_ReturnsNil(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	t.Setenv("LOG_PATHS", "")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.LogPaths) != 0 {
		t.Errorf("expected empty LogPaths, got: %v", cfg.LogPaths)
	}
}

// ── Defaults hardcoded ───────────────────────────────────────────────────────

func TestLoadAgent_Defaults_SendTimeout_10s(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SendTimeout != 10*time.Second {
		t.Errorf("expected SendTimeout=10s, got: %v", cfg.SendTimeout)
	}
}

func TestLoadAgent_Defaults_BackoffBase_1s(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BackoffBase != time.Second {
		t.Errorf("expected BackoffBase=1s, got: %v", cfg.BackoffBase)
	}
}

func TestLoadAgent_Defaults_BackoffMax_60s(t *testing.T) {
	t.Setenv("SERVER_URL", "http://localhost:8080")
	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BackoffMax != 60*time.Second {
		t.Errorf("expected BackoffMax=60s, got: %v", cfg.BackoffMax)
	}
}
