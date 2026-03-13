package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

type Consumer struct {
	client        *redis.Client
	streamName    string
	consumerGroup string
	consumerName  string
	logger        *observability.Logger
	dlq           *DLQ
}

func NewConsumer(client *redis.Client, streamName, consumerGroup, consumerName string, logger *observability.Logger, dlq *DLQ) *Consumer {
	return &Consumer{
		client:        client,
		streamName:    streamName,
		consumerGroup: consumerGroup,
		consumerName:  consumerName,
		logger:        logger,
		dlq:           dlq,
	}
}

func (c *Consumer) Consume(ctx context.Context, blockDuration time.Duration) (*JobEnvelope, string, error) {
	// No pending messages - block waiting for new ones
	streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    c.consumerGroup,
		Consumer: c.consumerName,
		Streams:  []string{c.streamName, ">"},
		Count:    1,
		Block:    blockDuration,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, "", nil
		}
		c.logger.Error("failed to read from stream", "error", err, "stream", c.streamName)
		return nil, "", fmt.Errorf("failed to read from stream: %w", err)
	}

	// Check if any messages returned
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return nil, "", nil
	}

	return c.parseMessage(streams[0].Messages[0])
}

// parseMessage extracts and deserializes a JobEnvelope from a stream message.
func (c *Consumer) parseMessage(msg redis.XMessage) (*JobEnvelope, string, error) {
	messageID := msg.ID
	jobStr, ok := msg.Values["job"].(string)
	if !ok {
		c.logger.Error("message missing data field", "message_id", messageID)
		return nil, messageID, fmt.Errorf("message missing data field")
	}
	var envelope JobEnvelope
	if err := json.Unmarshal([]byte(jobStr), &envelope); err != nil {
		c.logger.Error("failed to unmarshal log event",
			"error", err,
			"message_id", messageID,
		)
		return nil, messageID, fmt.Errorf("failed to unmarshal: %w", err)
	}
	c.logger.LogQueueDequeue(c.streamName, messageID, envelope.JobID, envelope.RetryCount)
	return &envelope, messageID, nil
}

func (c *Consumer) Ack(ctx context.Context, messageID string) error {
	err := c.client.XAck(ctx, c.streamName, c.consumerGroup, messageID).Err()
	if err != nil {
		c.logger.Error("failed to ack message",
			"error", err,
			"message_id", messageID,
		)
		return fmt.Errorf("failed to ack: %w", err)
	}

	c.logger.Debug("acked message", "message_id", messageID)
	return nil
}

// Retry increments retry count and re-enqueues, or sends to DLQ if max retries exceeded
func (c *Consumer) Retry(ctx context.Context, envelope *JobEnvelope, messageID string, lastError error) error {
	envelope.RetryCount++
	envelope.LastError = lastError.Error()

	// Check if exceeded max retries
	if envelope.RetryCount >= envelope.MaxRetries {
		c.logger.Warn("max retries exceeded, sending to DLQ",
			"job_id", envelope.JobID,
			"retry_count", envelope.RetryCount,
			"max_retries", envelope.MaxRetries,
		)

		if c.dlq != nil {
			dlqEntry := &DLQEntry{
				OriginalStreamID: messageID,
				JobData:          envelope,
				FailureReason:    lastError.Error(),
				SentToDLQAt:      time.Now().Unix(),
			}

			if _, err := c.dlq.Send(ctx, dlqEntry); err != nil {
				c.logger.Error("failed to send to DLQ", "error", err, "job_id", envelope.JobID)
				// Continue to ack anyway - don't want infinite retries
			}
		} else {
			c.logger.Error("max retries exceeded but no DLQ configured - dropping job", "job_id", envelope.JobID)
		}

		// Ack to remove from stream
		return c.Ack(ctx, messageID)
	}

	// Re-enqueue with incremented retry count
	args, _, err := buildXAddArgs(c.streamName, 0, envelope) // maxLen=0 for retry (don't trim)
	if err != nil {
		return fmt.Errorf("failed to build retry args: %w", err)
	}

	if err := c.client.XAdd(ctx, args).Err(); err != nil {
		c.logger.Error("failed to re-enqueue for retry", "error", err, "job_id", envelope.JobID)
		return fmt.Errorf("failed to re-enqueue: %w", err)
	}

	// Ack original message
	if err := c.Ack(ctx, messageID); err != nil {
		return err
	}

	c.logger.Info("job re-enqueued for retry",
		"job_id", envelope.JobID,
		"retry_count", envelope.RetryCount,
		"max_retries", envelope.MaxRetries,
	)

	return nil
}

// ReclaimStale reclaims ALL stale messages from dead consumers using XAUTOCLAIM
// Uses cursor pagination to handle large numbers of stale messages
func (c *Consumer) ReclaimStale(ctx context.Context, idleTime time.Duration, maxBatch int) ([]*JobEnvelope, []string, error) {
	var (
		allEnvelopes  []*JobEnvelope
		allMessageIDs []string
		cursor        = "0-0"
	)

	for {
		args := &redis.XAutoClaimArgs{
			Stream:   c.streamName,
			Group:    c.consumerGroup,
			Consumer: c.consumerName,
			MinIdle:  idleTime,
			Start:    cursor,
			Count:    int64(maxBatch),
		}
		messages, nextCursor, err := c.client.XAutoClaim(ctx, args).Result()
		if err != nil {
			if err == redis.Nil {
				break
			}
			c.logger.Error("failed to reclaim stale messages",
				"error", err,
				"stream", c.streamName,
				"idle_time", idleTime,
				"cursor", cursor,
			)
			return allEnvelopes, allMessageIDs, fmt.Errorf("failed to reclaim stale messages: %w", err)
		}

		for _, msg := range messages {
			envelope, messageID, err := c.parseMessage(msg)
			if err != nil {
				c.logger.Error("failed to parse reclaimed message",
					"error", err,
					"message_id", msg.ID,
				)
				continue
			}
			allEnvelopes = append(allEnvelopes, envelope)
			allMessageIDs = append(allMessageIDs, messageID)
		}

		if nextCursor == "0-0" {
			break
		}
		cursor = nextCursor

		select {
		case <-ctx.Done():
			return allEnvelopes, allMessageIDs, ctx.Err()
		default:
		}
	}

	if len(allEnvelopes) > 0 {
		c.logger.Info("reclaimed stale messages",
			"count", len(allEnvelopes),
			"stream", c.streamName,
			"idle_time", idleTime,
		)
	}
	return allEnvelopes, allMessageIDs, nil
}

// GetPendingInfo returns the count and age of oldest pending message
func (c *Consumer) GetPendingInfo(ctx context.Context) (count int64, oldestAgeSec int64, err error) {
	info, err := c.client.XPending(ctx, c.streamName, c.consumerGroup).Result()
	if err != nil {
		// Group might not exist yet or no pending messages
		if err == redis.Nil || err.Error() == "NOGROUP" {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("failed to get pending info: %w", err)
	}

	count = info.Count

	// Calculate age of oldest message
	if count > 0 && info.Lower != "" {
		// Parse message ID to get timestamp
		// Redis message IDs are in format: "timestamp-sequence"
		// Example: "1771561605582-0"
		parts := strings.Split(info.Lower, "-")
		if len(parts) > 0 {
			timestampMs, err := strconv.ParseInt(parts[0], 10, 64)
			if err == nil {
				nowMs := time.Now().UnixMilli()
				ageMs := nowMs - timestampMs
				oldestAgeSec = ageMs / 1000
			}
		}
	}

	return count, oldestAgeSec, nil
}
