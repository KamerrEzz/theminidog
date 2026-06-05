package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/kamerrezz/theminidog/internal/agent"
	"github.com/kamerrezz/theminidog/internal/agent/collector"
	"github.com/kamerrezz/theminidog/internal/agent/logtail"
	"github.com/kamerrezz/theminidog/internal/agent/sender"
	"github.com/kamerrezz/theminidog/internal/config"
)

// mintAgentToken creates a short-lived HS256 JWT signed with the given secret.
func mintAgentToken(secret string) (string, error) {
	claims := jwt.RegisteredClaims{
		Issuer:    "miniobserv-agent",
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func main() {
	cfg, err := config.LoadAgent()
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	reg := collector.NewRegistry(
		collector.NewCPUCollector(cfg.AgentHost),
		collector.NewMemoryCollector(cfg.AgentHost),
		collector.NewDiskCollector(cfg.AgentHost, cfg.DiskMounts),
		collector.NewNetworkCollector(cfg.AgentHost, cfg.NetIfaces),
	)

	var agentToken string
	if cfg.AgentToken != "" {
		tok, err := mintAgentToken(cfg.AgentToken)
		if err != nil {
			slog.Error("failed to mint agent token", "err", err)
			os.Exit(1)
		}
		agentToken = tok
	}

	snd := sender.NewHTTPSender(
		cfg.ServerURL.String()+"/api/v1/metrics",
		sender.BackoffConfig{
			Base:   cfg.BackoffBase,
			Max:    cfg.BackoffMax,
			Jitter: cfg.BackoffJitter,
		},
		slog.Default(),
	).WithToken(agentToken)

	ag := agent.New(reg, snd, agent.Options{
		Host:     cfg.AgentHost,
		Interval: cfg.CollectInterval,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if len(cfg.LogPaths) > 0 {
		logSnd := logtail.NewHTTPLogSender(
			cfg.ServerURL.String()+"/api/v1/logs",
			agentToken,
			sender.DefaultBackoff(),
			slog.Default(),
		)
		tailer, err := logtail.NewTailer(cfg.LogPaths, cfg.AgentHost, logSnd, slog.Default())
		if err != nil {
			slog.Error("failed to create log tailer", "err", err)
			os.Exit(1)
		}
		go tailer.Run(ctx)
	}

	slog.Info("agent starting", "host", cfg.AgentHost, "interval", cfg.CollectInterval, "server", cfg.ServerURL.String())
	ag.Run(ctx)
	slog.Info("agent shutdown complete")
}
