package queue

import "github.com/jeffreyzhai123/fluffy-dollop/pkg/models"

// JobEnvelope wraps a log event with metadata for queue processing
type JobEnvelope struct {
	JobID      string           `json:"job_id"`
	Data       *models.LogEvent `json:"data"`
	EnqueuedAt int64            `json:"enqueued_at"`
	RetryCount int              `json:"retry_count"`
	MaxRetries int              `json:"max_retries"`
	EventType  string           `json:"event_type"`
	LastError  string           `json:"last_error,omitempty"`
}

// DLQEntry represents a failed job sent to the dead letter queue
type DLQEntry struct {
	OriginalStreamID string       `json:"original_stream_id"` // Redis stream message ID
	JobData          *JobEnvelope `json:"job_data"`           // Complete original job
	FailureReason    string       `json:"failure_reason"`     // Last error message
	SentToDLQAt      int64        `json:"sent_to_dlq_at"`     // Unix timestamp
}
