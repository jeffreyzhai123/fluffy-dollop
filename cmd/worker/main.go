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
	"github.com/jeffreyzhai123/fluffy-dollop/internal/worker"
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

	logger := observability.NewLogger(&cfg.Observability, "worker-service")
	logger.Info("initiating worker service startup")

	// Generate workerID once — shared by consumer, registry, and worker
	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("worker-%s-%d", hostname, os.Getpid())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Redis
	redisClient, err := bootstrap.SetupRedis(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer redisClient.Close()

	// Partitions & Consumer group
	if err := bootstrap.SetupPartitions(ctx, redisClient, cfg, logger); err != nil {
		return fmt.Errorf("failed to setup partitions: %w", err)
	}

	// Queue layer
	dlq := bootstrap.SetupDLQ(redisClient, cfg, logger)
	consumer, err := bootstrap.SetupConsumer(ctx, redisClient, dlq, cfg, logger, workerID)
	if err != nil {
		return fmt.Errorf("failed to setup consumer: %w", err)
	}

	// Registry — register on startup, deregister on shutdown
	reg := bootstrap.SetupRegistry(redisClient, cfg, logger)
	if err := reg.Register(ctx, workerID); err != nil {
		return fmt.Errorf("failed to register worker: %w", err)
	}
	defer reg.Deregister(context.Background(), workerID)

	// Health
	healthAgg := bootstrap.SetupHealthChecks(redisClient, cfg, logger)
	healthServer := worker.NewHealthServer(cfg, healthAgg, logger)

	// Processing
	processor := worker.NewProcessor(logger)
	dedupProcessor := worker.NewDeduplicatingProcessor(processor, redisClient, cfg.Worker.DedupTTL, logger)
	wrk := worker.NewWorker(consumer, dedupProcessor, &cfg.Worker, &cfg.Registry, cfg.Queue.PartitionID,
		logger, healthServer, workerID, reg, redisClient)

	if err := wrk.Start(ctx); err != nil {
		return fmt.Errorf("worker failed: %w", err)
	}

	logger.Info("shutdown completed successfully")
	return nil
}
