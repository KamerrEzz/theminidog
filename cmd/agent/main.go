package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kamerrezz/theminidog/internal/agent"
	"github.com/kamerrezz/theminidog/internal/agent/collector"
	"github.com/kamerrezz/theminidog/internal/agent/sender"
	"github.com/kamerrezz/theminidog/internal/config"
)

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

	snd := sender.NewHTTPSender(
		cfg.ServerURL.String()+"/api/v1/metrics",
		sender.BackoffConfig{
			Base:   cfg.BackoffBase,
			Max:    cfg.BackoffMax,
			Jitter: cfg.BackoffJitter,
		},
		slog.Default(),
	)

	ag := agent.New(reg, snd, agent.Options{
		Host:     cfg.AgentHost,
		Interval: cfg.CollectInterval,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("agent starting", "host", cfg.AgentHost, "interval", cfg.CollectInterval, "server", cfg.ServerURL.String())
	ag.Run(ctx)
	slog.Info("agent shutdown complete")
}
