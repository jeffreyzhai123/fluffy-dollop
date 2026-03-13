package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDLQ_MaxRetriesExceeded is the end-to-end entry point:
// it proves that the consumer/worker pipeline routes an exhausted
// job to the DLQ and removes it from the main stream's pending set.
// (The detailed retry path is covered in retry_test.go;
// this test focuses specifically on the DLQ entry contents.)
func TestDLQ_MaxRetriesExceeded(t *testing.T) {
	s := newSuite(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processor := testutil.NewFailingProcessor(s.Logger)

	_, err := s.Producer.Enqueue(context.Background(), &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "WARN",
		Service:   "max-retry-svc",
		Message:   "job that will be sent to DLQ",
	})
	require.NoError(t, err)

	wrk := s.newWorker(t, ctx, processor)
	done := make(chan error, 1)
	go func() { done <- wrk.Start(ctx) }()

	select {
	case <-wrk.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not become ready")
	}

	// Wait: MaxRetries+1 process calls then one DLQ entry
	expectedCalls := int64(s.Config.Worker.JobMaxRetries)
	testutil.WaitFor(t, 30*time.Second, func() bool {
		return processor.ProcessCount() >= expectedCalls
	}, fmt.Sprintf("processor called at least %d times", expectedCalls))

	testutil.WaitFor(t, 5*time.Second, func() bool {
		count, err := s.DLQ.Count(context.Background())
		return err == nil && count == 1
	}, "one entry in DLQ")

	// Inspect the DLQ entry — it must carry the failure reason and original data
	entries, err := s.DLQ.List(context.Background(), "-", "+", 10)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	entry := entries[0]
	assert.NotEmpty(t, entry.FailureReason, "failure reason should be populated")
	assert.NotNil(t, entry.JobData, "original job data should be preserved")
	assert.Equal(t, s.Config.Worker.JobMaxRetries, entry.JobData.MaxRetries,
		"MaxRetries on envelope should match config")
	assert.Greater(t, entry.SentToDLQAt, int64(0), "SentToDLQAt should be set")

	// Main stream must have no pending messages
	pendingCount, err := testutil.GetPendingCount(s.Redis,
		s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pendingCount)

	cancel()
	testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
}

// TestDLQ_Requeue verifies that a DLQ entry can be moved back to
// the main stream and subsequently processed by a worker.
func TestDLQ_Requeue(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()

	// Directly send a DLQ entry (bypassing the retry path — tested elsewhere)
	entry := &queue.DLQEntry{
		OriginalStreamID: "original-msg-id",
		FailureReason:    "manual test injection",
		SentToDLQAt:      time.Now().UnixMilli(),
		JobData: &queue.JobEnvelope{
			JobID:      "requeue-test-job",
			MaxRetries: s.Config.Worker.JobMaxRetries,
			RetryCount: s.Config.Worker.JobMaxRetries, // exhausted
			EventType:  "log",
			EnqueuedAt: time.Now().UnixMilli(),
			Data: &models.LogEvent{
				Timestamp: time.Now(),
				Level:     "INFO",
				Service:   "requeue-svc",
				Message:   "requeueing from DLQ",
			},
		},
	}

	dlqMsgID, err := s.DLQ.Send(ctx, entry)
	require.NoError(t, err, "failed to send entry to DLQ")
	require.NotEmpty(t, dlqMsgID)

	countBefore, err := s.DLQ.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), countBefore, "DLQ should have one entry before requeue")

	// Requeue back to main stream
	err = s.DLQ.Requeue(ctx, dlqMsgID, s.Config.Queue.BaseStreamName, s.Config.Queue.MaxLen)
	require.NoError(t, err, "requeue should succeed")

	// Entry should be gone from DLQ
	testutil.WaitFor(t, 5*time.Second, func() bool {
		count, err := s.DLQ.Count(ctx)
		return err == nil && count == 0
	}, "DLQ to be empty after requeue")

	// Message should be visible in the main stream
	testutil.WaitForStreamLengthAtLeast(t, s.Redis, s.Config.Queue.BaseStreamName, 1, 5*time.Second)

	// Start a worker and verify the requeued job is processed
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	processor := testutil.NewSucceedingProcessor(s.Logger)
	wrk := s.newWorker(t, workerCtx, processor)
	done := make(chan error, 1)
	go func() { done <- wrk.Start(workerCtx) }()

	select {
	case <-wrk.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not become ready")
	}

	testutil.WaitFor(t, 10*time.Second, func() bool {
		return processor.ProcessCount() == 1
	}, "requeued job to be processed")

	assert.Equal(t, int64(1), processor.ProcessCount())

	cancel()
	testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
}
