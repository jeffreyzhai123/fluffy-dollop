package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// DLQ handles dead letter queue operations
type DLQ struct {
	client     *redis.Client
	streamName string
	maxLen     int64
	logger     *observability.Logger
}

// NewDLQ creates a new dead letter queue handler
func NewDLQ(client *redis.Client, streamName string, maxLen int64, logger *observability.Logger) *DLQ {
	return &DLQ{
		client:     client,
		streamName: streamName,
		maxLen:     maxLen,
		logger:     logger,
	}
}

// Send adds a failed job to the DLQ
func (d *DLQ) Send(ctx context.Context, entry *DLQEntry) (string, error) {

	// Serialize entry
	entryJSON, err := json.Marshal(entry)
	if err != nil {
		d.logger.Error("failed to marshal DLQ entry",
			"error", err,
			"job_id", entry.JobData.JobID,
		)
		return "", fmt.Errorf("failed to marshal DLQ entry: %w", err)
	}

	// Add to DLQ stream with trimming
	args := &redis.XAddArgs{
		Stream: d.streamName,
		MaxLen: d.maxLen,
		Approx: true, // Use ~ for efficiency
		Values: map[string]interface{}{
			"entry": string(entryJSON),
		},
	}

	messageID, err := d.client.XAdd(ctx, args).Result()
	if err != nil {
		d.logger.Error("failed to add to DLQ",
			"error", err,
			"job_id", entry.JobData.JobID,
			"stream", d.streamName,
		)
		return "", fmt.Errorf("failed to add to DLQ: %w", err)
	}

	d.logger.Warn("job sent to DLQ",
		"dlq_message_id", messageID,
		"original_stream_id", entry.OriginalStreamID,
		"job_id", entry.JobData.JobID,
		"failure_reason", entry.FailureReason,
		"retry_count", entry.JobData.RetryCount,
		"max_retries", entry.JobData.MaxRetries,
	)

	return messageID, nil
}

// Get retrieves a DLQ entry by message ID
func (d *DLQ) Get(ctx context.Context, messageID string) (*DLQEntry, error) {
	messages, err := d.client.XRange(ctx, d.streamName, messageID, messageID).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get DLQ entry: %w", err)
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("DLQ entry not found: %s", messageID)
	}

	entryJSON, ok := messages[0].Values["entry"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid DLQ entry format")
	}

	var entry DLQEntry
	if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal DLQ entry: %w", err)
	}

	return &entry, nil
}

// List retrieves DLQ entries with pagination
func (d *DLQ) List(ctx context.Context, start, end string, count int64) ([]*DLQEntry, error) {
	messages, err := d.client.XRange(ctx, d.streamName, start, end).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list DLQ entries: %w", err)
	}

	// Apply count limit if specified
	if count > 0 && int64(len(messages)) > count {
		messages = messages[:count]
	}

	entries := make([]*DLQEntry, 0, len(messages))
	for _, msg := range messages {
		entryJSON, ok := msg.Values["entry"].(string)
		if !ok {
			d.logger.Warn("skipping invalid DLQ entry", "message_id", msg.ID)
			continue
		}

		var entry DLQEntry
		if err := json.Unmarshal([]byte(entryJSON), &entry); err != nil {
			d.logger.Warn("skipping malformed DLQ entry",
				"message_id", msg.ID,
				"error", err,
			)
			continue
		}

		entries = append(entries, &entry)
	}

	return entries, nil
}

// Count returns the number of entries in the DLQ
func (d *DLQ) Count(ctx context.Context) (int64, error) {
	count, err := d.client.XLen(ctx, d.streamName).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get DLQ count: %w", err)
	}
	return count, nil
}

// Delete removes a DLQ entry by message ID
func (d *DLQ) Delete(ctx context.Context, messageID string) error {
	deleted, err := d.client.XDel(ctx, d.streamName, messageID).Result()
	if err != nil {
		return fmt.Errorf("failed to delete DLQ entry: %w", err)
	}

	if deleted == 0 {
		return fmt.Errorf("DLQ entry not found: %s", messageID)
	}

	d.logger.Debug("deleted DLQ entry", "message_id", messageID)
	return nil
}

// Requeue moves a DLQ entry back to the main queue for reprocessing
func (d *DLQ) Requeue(ctx context.Context, messageID string, mainStreamName string, maxLen int64) error {
	// Get the DLQ entry
	entry, err := d.Get(ctx, messageID)
	if err != nil {
		return fmt.Errorf("failed to get DLQ entry: %w", err)
	}

	// Reset retry count and error for reprocessing
	entry.JobData.RetryCount = 0
	entry.JobData.LastError = ""

	// Build args for re-enqueueing to main stream
	args, _, err := buildXAddArgs(mainStreamName, maxLen, entry.JobData)
	if err != nil {
		return fmt.Errorf("failed to build requeue args: %w", err)
	}

	// Add back to main stream
	newMessageID, err := d.client.XAdd(ctx, args).Result()
	if err != nil {
		return fmt.Errorf("failed to requeue to main stream: %w", err)
	}

	// Delete from DLQ
	if err := d.Delete(ctx, messageID); err != nil {
		d.logger.Warn("failed to delete DLQ entry after requeue",
			"dlq_message_id", messageID,
			"error", err,
		)
		// Don't fail - job is already requeued
	}

	d.logger.Info("requeued job from DLQ",
		"dlq_message_id", messageID,
		"new_message_id", newMessageID,
		"job_id", entry.JobData.JobID,
	)

	return nil
}
