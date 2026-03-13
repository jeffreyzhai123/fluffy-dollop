package bootstrap

import (
	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// SetupProducer creates a queue producer for ingestion
func SetupProducer(client *redis.Client, cfg *config.Config, logger *observability.Logger) queue.ProducerInterface {
	producer := queue.NewProducer(client, &cfg.Queue, logger)
	logger.Info("producer initialized",
		"stream", cfg.Queue.BaseStreamName,
		"partition_count", cfg.Queue.PartitionCount,
		"max_len", cfg.Queue.MaxLen,
	)
	return producer
}
