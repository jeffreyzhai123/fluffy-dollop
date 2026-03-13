package queue

import (
	"context"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
)

// ProducerInterface defines the contract for enqueueing jobs
type ProducerInterface interface {
	Enqueue(ctx context.Context, logEvent *models.LogEvent) (string, error)
	EnqueueBatch(ctx context.Context, logEvents []*models.LogEvent) ([]string, error)
	StreamLength(ctx context.Context) (int64, error)
	Ping(ctx context.Context) error
}

// ConsumerInterface defines the contract for consuming jobs
type ConsumerInterface interface {
	Consume(ctx context.Context, blockDuration time.Duration) (*JobEnvelope, string, error)
	Ack(ctx context.Context, messageID string) error
	Retry(ctx context.Context, envelope *JobEnvelope, messageID string, lastError error) error
	ReclaimStale(ctx context.Context, idleTime time.Duration, maxBatch int) ([]*JobEnvelope, []string, error)
	GetPendingInfo(ctx context.Context) (count int64, oldestAge int64, err error)
}
type DLQInterface interface {
	Send(ctx context.Context, entry *DLQEntry) (string, error)
	Get(ctx context.Context, messageID string) (*DLQEntry, error)
	List(ctx context.Context, start, end string, count int64) ([]*DLQEntry, error)
	Count(ctx context.Context) (int64, error)
	Delete(ctx context.Context, messageID string) error
	Requeue(ctx context.Context, messageID string, mainStreamName string, maxLen int64) error
}

// Verify implementations satisfy interfaces at compile time
var _ ProducerInterface = (*Producer)(nil)
var _ ConsumerInterface = (*Consumer)(nil)
var _ DLQInterface = (*DLQ)(nil)
