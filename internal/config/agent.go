package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// AgentConfig holds all runtime configuration for the metrics agent.
type AgentConfig struct {
	ServerURL       *url.URL
	AgentHost       string
	AgentToken      string
	CollectInterval time.Duration
	SendTimeout     time.Duration
	DiskMounts      []string
	NetIfaces       []string
	BackoffBase     time.Duration
	BackoffMax      time.Duration
	BackoffJitter   float64
	LogPaths        []string
	LogLevel        string
}

// LoadAgent reads AgentConfig from environment variables.
// Returns an error (fail-fast) only for SERVER_URL issues.
// All other out-of-range or invalid values fall back to safe defaults.
func LoadAgent() (AgentConfig, error) {
	rawURL := os.Getenv("SERVER_URL")
	if rawURL == "" {
		return AgentConfig{}, fmt.Errorf("SERVER_URL is required but not set")
	}
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return AgentConfig{}, fmt.Errorf("SERVER_URL must be a valid http/https URL, got %q", rawURL)
	}

	host := os.Getenv("AGENT_HOST")
	if host == "" {
		host, _ = os.Hostname()
	}

	interval := 10 * time.Second
	if raw := os.Getenv("COLLECT_INTERVAL"); raw != "" {
		if d, parseErr := time.ParseDuration(raw); parseErr == nil && d >= time.Second && d <= 300*time.Second {
			interval = d
		}
		// out-of-range or parse error: silently fall back to default
	}

	logLevel := "info"
	if ll := os.Getenv("LOG_LEVEL"); ll == "warn" || ll == "error" || ll == "debug" {
		logLevel = ll
	}

	var logPaths []string
	if raw := os.Getenv("LOG_PATHS"); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(p); t != "" {
				logPaths = append(logPaths, t)
			}
		}
	}

	agentToken := os.Getenv("AGENT_TOKEN")
	// optional — empty is valid

	return AgentConfig{
		ServerURL:       u,
		AgentHost:       host,
		AgentToken:      agentToken,
		CollectInterval: interval,
		SendTimeout:     10 * time.Second,
		DiskMounts:      nil,
		NetIfaces:       nil,
		BackoffBase:     time.Second,
		BackoffMax:      60 * time.Second,
		BackoffJitter:   0.25,
		LogPaths:        logPaths,
		LogLevel:        logLevel,
	}, nil
}
