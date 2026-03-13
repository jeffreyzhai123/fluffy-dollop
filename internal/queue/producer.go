package queue

import (
	"context"
	"fmt"
	"hash/fnv"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
)

type Producer struct {
	client *redis.Client
	config *config.QueueConfig
	logger *observability.Logger
}

func NewProducer(client *redis.Client, cfg *config.QueueConfig, logger *observability.Logger) *Producer {
	return &Producer{
		client: client,
		config: cfg,
		logger: logger,
	}
}

// Total length across all partitions
func (p *Producer) StreamLength(ctx context.Context) (int64, error) {
	var total int64
	for i := 0; i < p.config.PartitionCount; i++ {
		streamName := fmt.Sprintf("%s:partition:%d", p.config.BaseStreamName, i)
		length, err := p.client.XLen(ctx, streamName).Result()
		if err != nil {
			p.logger.Error("failed to get stream length",
				"error", err,
				"stream", streamName,
			)
			return 0, fmt.Errorf("failed to get stream length for partition %d: %w", i, err)
		}
		total += length
	}
	return total, nil
}

func (p *Producer) Enqueue(ctx context.Context, logEvent *models.LogEvent) (string, error) {
	streamName := p.streamNameFor(logEvent.Service)
	envelope := buildJobEnvelope(logEvent, p.config.RedisMaxRetries)
	args, payloadSize, err := buildXAddArgs(streamName, p.config.MaxLen, envelope)
	if err != nil {
		p.logger.Error("failed to build XADD args", "error", err)
		return "", err
	}

	result := p.client.XAdd(ctx, args)
	if err := result.Err(); err != nil {
		p.logger.Error("failed to execute XADD",
			"error", err,
			"stream", streamName,
		)
		return "", fmt.Errorf("failed to add to stream: %w", err)
	}

	messageID := result.Val()
	p.logger.LogQueueEnqueue(streamName, messageID, payloadSize)
	return messageID, nil
}

// EnqueueBatch enqueues multiple log events in a single pipeline
func (p *Producer) EnqueueBatch(ctx context.Context, logEvents []*models.LogEvent) ([]string, error) {
	if len(logEvents) == 0 {
		return []string{}, nil
	}

	pipe := p.client.Pipeline()
	cmds := make([]*redis.StringCmd, 0, len(logEvents))

	for _, logEvent := range logEvents {
		streamName := p.streamNameFor(logEvent.Service)
		envelope := buildJobEnvelope(logEvent, p.config.RedisMaxRetries)
		args, _, err := buildXAddArgs(streamName, p.config.MaxLen, envelope)
		if err != nil {
			p.logger.Error("failed to build XADD args in batch", "error", err)
			return nil, err
		}
		cmd := pipe.XAdd(ctx, args)
		cmds = append(cmds, cmd)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		p.logger.Error("failed to execute batch XADD",
			"error", err,
			"batch_size", len(logEvents),
		)
		return nil, fmt.Errorf("failed to execute batch: %w", err)
	}

	messageIDs := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		messageIDs = append(messageIDs, cmd.Val())
	}

	p.logger.Info("enqueued batch",
		"count", len(messageIDs),
	)
	return messageIDs, nil
}

// partitionFor returns the partition ID for a given routing key.
// Uses FNV-1a hash for even distribution without external dependencies.
func (p *Producer) partitionFor(routingKey string) int {
	h := fnv.New32a()
	h.Write([]byte(routingKey))
	return int(h.Sum32()) % p.config.PartitionCount
}

// streamNameFor returns the full stream name for a routing key.
func (p *Producer) streamNameFor(routingKey string) string {
	partitionID := p.partitionFor(routingKey)
	return fmt.Sprintf("%s:partition:%d", p.config.BaseStreamName, partitionID)
}

func (p *Producer) Ping(ctx context.Context) error {
	return p.client.Ping(ctx).Err()
}
