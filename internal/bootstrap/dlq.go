package bootstrap

import (
	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// SetupDLQ creates a dead letter queue
func SetupDLQ(
	client *redis.Client,
	cfg *config.Config,
	logger *observability.Logger,
) *queue.DLQ {
	return queue.NewDLQ(
		client,
		cfg.Queue.DLQStreamName,
		cfg.Queue.DLQMaxLen,
		logger,
	)
}
