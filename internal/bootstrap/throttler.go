package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/ingestion"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// SetupThrottler creates and starts an adaptive throttler for ingestion
func SetupThrottler(
	ctx context.Context,
	producer queue.ProducerInterface,
	redisClient *redis.Client,
	dlq *queue.DLQ,
	cfg *config.Config,
	logger *observability.Logger,
) ingestion.ThrottlerInterface {
	// If throttler is disabled, return noop
	if !cfg.Ingestion.Throttler.Enabled {
		logger.Info("throttler disabled, using noop throttler")
		return &ingestion.NoopThrottler{}
	}

	// Throttler gets its own consumer identity — it's a read-only observer,
	// not a processing worker, so it should not share a workerID with workers
	hostname, _ := os.Hostname()
	throttlerConsumerID := fmt.Sprintf("throttler-%s-%d", hostname, os.Getpid())

	// Setup consumer for pending message checks
	consumer, err := SetupConsumer(ctx, redisClient, dlq, cfg, logger, throttlerConsumerID)
	if err != nil {
		logger.Warn("failed to setup consumer for throttler, using limited throttling", "error", err)
		consumer = nil // Throttler will work without pending metrics
	}

	// Create throttler
	throttler := ingestion.NewThrottler(producer, consumer, &cfg.Ingestion.Throttler, logger)

	// Start background metrics updater
	throttler.Start(ctx)

	return throttler
}
