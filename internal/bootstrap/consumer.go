package bootstrap

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

func SetupConsumer(
	ctx context.Context,
	client *redis.Client,
	dlq *queue.DLQ,
	cfg *config.Config,
	logger *observability.Logger,
	workerID string,
) (queue.ConsumerInterface, error) {
	streamName := PartitionStreamName(cfg.Queue.BaseStreamName, cfg.Queue.PartitionID)
	groupName := PartitionGroupName(cfg.Queue.BaseConsumerGroup, cfg.Queue.PartitionID)

	logger.Info("consumer group ready",
		"stream", streamName,
		"group", groupName,
		"consumer", workerID,
	)

	consumer := queue.NewConsumer(client, streamName, groupName, workerID, logger, dlq)
	return consumer, nil
}
