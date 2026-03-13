package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/redis/go-redis/v9"
)

// SetupPartitions creates all partition streams and consumer groups at startup.
// Safe to call on every worker start — BUSYGROUP errors are ignored.
// Run this before SetupConsumer so streams exist before workers poll them.
func SetupPartitions(
	ctx context.Context,
	client *redis.Client,
	cfg *config.Config,
	logger *observability.Logger,
) error {
	for i := 0; i < cfg.Queue.PartitionCount; i++ {
		streamName := PartitionStreamName(cfg.Queue.BaseStreamName, i)
		groupName := PartitionGroupName(cfg.Queue.BaseConsumerGroup, i)

		err := client.XGroupCreateMkStream(ctx, streamName, groupName, "0").Err()
		if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
			return fmt.Errorf("failed to create partition %d (stream=%s group=%s): %w",
				i, streamName, groupName, err)
		}

		logger.Info("partition ready",
			"partition_id", i,
			"stream", streamName,
			"group", groupName,
		)
	}
	return nil
}

// PartitionStreamName returns the stream name for a given partition.
func PartitionStreamName(baseStream string, partitionID int) string {
	return fmt.Sprintf("%s:partition:%d", baseStream, partitionID)
}

// PartitionGroupName returns the consumer group name for a given partition.
func PartitionGroupName(baseGroup string, partitionID int) string {
	return fmt.Sprintf("%s-%d", baseGroup, partitionID)
}
