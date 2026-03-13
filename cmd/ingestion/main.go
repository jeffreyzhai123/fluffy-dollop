package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/bootstrap"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/ingestion"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// entry point for ingestion service
func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	// Successful exit
}

// run contains the main application logic
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	logger := observability.NewLogger(&cfg.Observability, "ingestion-service")
	logger.Info("initiating ingestion service startup")

	// Setup graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Setup Redis client
	redisClient, err := bootstrap.SetupRedis(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	// Setup Producer
	producer := bootstrap.SetupProducer(redisClient, cfg, logger)

	// Setup DLQ
	dlq := bootstrap.SetupDLQ(redisClient, cfg, logger)

	// Setup throttler
	throttler := bootstrap.SetupThrottler(ctx, producer, redisClient, dlq, cfg, logger)
	defer throttler.Stop()

	// Setup health checks
	healthAgg := bootstrap.SetupHealthChecks(redisClient, cfg, logger)

	// Setup Ingestion Service and HTTP Server
	service := ingestion.NewService(producer, cfg, logger, throttler)
	server := ingestion.NewServer(cfg, logger, healthAgg, service)

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("server stopped with error: %w", err)
	}

	logger.Info("ingestion service shutdown completed successfully")
	return nil
}
