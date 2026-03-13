package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/bootstrap"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/registry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := observability.NewLogger(&cfg.Observability, "controlplane")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	redisClient, err := bootstrap.SetupRedis(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	defer redisClient.Close()

	reg := registry.NewRegistry(redisClient, &cfg.Registry, logger)
	cp := registry.NewControlPlane(reg, &cfg.Registry, logger)

	logger.Info("starting control plane")
	if err := cp.Start(ctx); err != nil {
		return fmt.Errorf("control plane exited with error: %w", err)
	}

	logger.Info("control plane stopped cleanly")
	return nil
}
