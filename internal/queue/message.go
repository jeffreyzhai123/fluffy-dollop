package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/redis/go-redis/v9"
)

// buildJobEnvelope creates a job envelope from log event JSON
func buildJobEnvelope(logEvent *models.LogEvent, maxRetries int) *JobEnvelope {
	envelope := &JobEnvelope{
		JobID:      uuid.New().String(),
		Data:       logEvent,
		EnqueuedAt: time.Now().Unix(),
		RetryCount: 0,
		MaxRetries: maxRetries,
		EventType:  "log_event",
	}
	return envelope
}

// buildXAddArgs constructs Redis XAddArgs from a job envelope
func buildXAddArgs(streamName string, maxLen int64, envelope *JobEnvelope) (*redis.XAddArgs, int, error) {
	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal job envelope: %w", err)
	}

	args := &redis.XAddArgs{
		Stream: streamName,
		MaxLen: maxLen,
		Approx: true,
		ID:     "*",
		Values: map[string]interface{}{
			"job": string(envelopeJSON),
		},
	}

	return args, len(envelopeJSON), nil
}

func parseMessage(data string) (*models.LogEvent, error) {
	var logEvent models.LogEvent
	if err := json.Unmarshal([]byte(data), &logEvent); err != nil {
		return nil, err
	}
	return &logEvent, nil
}
