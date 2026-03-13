package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/worker"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeduplication_SameJobIDProcessedOnce verifies that when the same job
// payload is delivered twice (simulating a crash-before-ack scenario), the
// downstream processor is only called once and the second delivery is skipped.
//
// We inject the duplicate via raw XAdd rather than calling Enqueue twice,
// because Enqueue stamps a fresh UUID as JobID on every call. The raw approach
// copies the exact payload of the first message — the same JobID — which is
// what actually happens in production when XAUTOCLAIM or retry redelivers.
func TestDeduplication_SameJobIDProcessedOnce(t *testing.T) {
	s := newSuite(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := testutil.NewSucceedingProcessor(s.Logger)
	dedup := worker.NewDeduplicatingProcessor(inner, s.Redis, 24*time.Hour, s.Logger)

	// Enqueue the original message
	firstMsgID, err := s.Producer.Enqueue(ctx, &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "dedup-test",
		Message:   "this event should only be processed once",
	})
	require.NoError(t, err, "first enqueue failed")
	require.NotEmpty(t, firstMsgID)

	// Read the raw payload so we can inject an exact duplicate.
	// This mirrors production redelivery — same JobID, new stream message ID.
	messages, err := s.Redis.XRange(ctx,
		s.Config.Queue.BaseStreamName, firstMsgID, firstMsgID).Result()
	require.NoError(t, err, "failed to read first message")
	require.Len(t, messages, 1, "should find exactly one message")

	// Inject duplicate with identical job payload (same JobID inside envelope)
	_, err = s.Redis.XAdd(ctx, &redis.XAddArgs{
		Stream: s.Config.Queue.BaseStreamName,
		Values: messages[0].Values,
	}).Result()
	require.NoError(t, err, "failed to inject duplicate message")

	wrk := s.newWorker(t, ctx, dedup)
	done := make(chan error, 1)
	go func() { done <- wrk.Start(ctx) }()

	select {
	case <-wrk.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not become ready")
	}

	// Wait for dedup layer to see both deliveries (processed + skipped = 2)
	testutil.WaitFor(t, 10*time.Second, func() bool {
		total := inner.ProcessCount() + dedup.DuplicateCount()
		return total == 2
	}, "both deliveries to be seen by dedup layer")

	// Downstream should only have run once
	assert.Equal(t, int64(1), inner.ProcessCount(),
		"downstream processor should only run once for duplicate JobID")

	// Dedup should have caught exactly one duplicate
	assert.Equal(t, int64(1), dedup.DuplicateCount(),
		"dedup should have skipped exactly one duplicate")

	// Both messages acked — no pending regardless of dedup outcome
	testutil.WaitForPendingCount(t, s.Redis,
		s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup,
		0, 5*time.Second)

	cancel()
	testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
}

// TestDeduplication_DifferentJobIDsBothProcessed verifies that distinct jobs
// are not incorrectly deduplicated — each unique JobID must reach downstream.
func TestDeduplication_DifferentJobIDsBothProcessed(t *testing.T) {
	s := newSuite(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := testutil.NewSucceedingProcessor(s.Logger)
	dedup := worker.NewDeduplicatingProcessor(inner, s.Redis, 24*time.Hour, s.Logger)

	const numJobs = 5
	for i := 0; i < numJobs; i++ {
		_, err := s.Producer.Enqueue(ctx, &models.LogEvent{
			Timestamp: time.Now(),
			Level:     "INFO",
			Service:   "dedup-test",
			Message:   "unique event",
			Metadata: map[string]interface{}{
				"index": i,
			},
		})
		require.NoError(t, err, "failed to enqueue job %d", i)
	}

	wrk := s.newWorker(t, ctx, dedup)
	done := make(chan error, 1)
	go func() { done <- wrk.Start(ctx) }()

	select {
	case <-wrk.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not become ready")
	}

	testutil.WaitFor(t, 10*time.Second, func() bool {
		return inner.ProcessCount() == int64(numJobs)
	}, "all distinct jobs to be processed downstream")

	assert.Equal(t, int64(numJobs), inner.ProcessCount(),
		"all distinct jobs should reach downstream processor")
	assert.Equal(t, int64(0), dedup.DuplicateCount(),
		"no duplicates should be detected for distinct jobs")

	cancel()
	testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
}
