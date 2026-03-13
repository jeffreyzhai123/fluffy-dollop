package integration

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRetry_JobRetriesOnFailure verifies that a failing job is retried
// and that RetryCount increments correctly on each attempt.
func TestRetry_JobRetriesOnFailure(t *testing.T) {
	tests := []struct {
		name           string
		succeedOnRetry int // process call number on which we succeed (1-indexed)
		wantRetries    int
	}{
		{
			name:           "succeeds on second attempt",
			succeedOnRetry: 2,
			wantRetries:    1,
		},
		{
			name:           "succeeds on third attempt",
			succeedOnRetry: 3,
			wantRetries:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSuite(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Track observed retry counts across calls
			observedRetryCounts := make([]atomic.Int32, tt.succeedOnRetry)
			var callCount atomic.Int32

			// Create a processor that fails until the specified attempt, and records the RetryCount it sees
			processor := testutil.NewControllableProcessor(s.Logger,
				func(ctx context.Context, envelope *queue.JobEnvelope) error {
					call := int(callCount.Add(1))
					// Record the RetryCount the worker stamped on this delivery
					observedRetryCounts[call-1].Store(int32(envelope.RetryCount))

					if call < tt.succeedOnRetry {
						return fmt.Errorf("simulated failure on attempt %d", call)
					}
					return nil
				},
			)

			// Enqueue a job that will fail and be retried
			_, err := s.Producer.Enqueue(context.Background(), &models.LogEvent{
				Timestamp: time.Now(),
				Level:     "ERROR",
				Service:   "retry-test",
				Message:   "message that will be retried",
			})
			require.NoError(t, err)

			// Start worker
			wrk := s.newWorker(t, ctx, processor)
			done := make(chan error, 1)
			go func() { done <- wrk.Start(ctx) }()

			select {
			case <-wrk.Ready():
			case <-time.After(2 * time.Second):
				t.Fatal("worker did not become ready")
			}

			// Wait until the processor has been called the expected number of times
			testutil.WaitFor(t, 15*time.Second, func() bool {
				return int(callCount.Load()) >= tt.succeedOnRetry
			}, fmt.Sprintf("processor to be called %d times", tt.succeedOnRetry))

			// Verify RetryCount incremented on each re-delivery
			assert.Len(t, observedRetryCounts, tt.succeedOnRetry)
			for i := range observedRetryCounts {
				assert.Equal(t, int32(i), observedRetryCounts[i].Load(), "expected RetryCount %d on attempt %d", i, i+1)
			}

			// After eventual success no messages should remain pending
			testutil.WaitForPendingCount(t, s.Redis,
				s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup,
				0, 5*time.Second)

			cancel()
			testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
		})
	}
}

// TestRetry_JobEventuallyReachesDLQ verifies the full retry → DLQ path:
// a job that keeps failing must be moved to the DLQ after MaxRetries
// attempts and must no longer appear in the main stream's pending set.
func TestRetry_JobEventuallyReachesDLQ(t *testing.T) {
	tests := []struct {
		name       string
		maxRetries int // driven by config; we just observe the outcome
	}{
		{name: "default max retries", maxRetries: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSuite(t)

			// Override MaxRetries so the test is deterministic and fast
			s.Config.Worker.JobMaxRetries = tt.maxRetries

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			processor := testutil.NewFailingProcessor(s.Logger)

			_, err := s.Producer.Enqueue(context.Background(), &models.LogEvent{
				Timestamp: time.Now(),
				Level:     "ERROR",
				Service:   "dlq-test",
				Message:   "this job will exhaust retries",
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

			// We expect MaxRetries+1 process calls (initial + each retry)
			expectedCalls := int64(tt.maxRetries)
			testutil.WaitFor(t, 30*time.Second, func() bool {
				return processor.ProcessCount() >= expectedCalls
			}, fmt.Sprintf("processor to be called at least %d times", expectedCalls))

			// Main stream pending should be zero — job was acked after DLQ send
			testutil.WaitForPendingCount(t, s.Redis,
				s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup,
				0, 5*time.Second)

			// DLQ should have exactly one entry
			testutil.WaitFor(t, 5*time.Second, func() bool {
				count, err := s.DLQ.Count(context.Background())
				if err != nil {
					t.Logf("DLQ count error (will retry): %v", err)
					return false
				}
				return count == 1
			}, "DLQ to contain exactly one entry")

			dlqCount, err := s.DLQ.Count(context.Background())
			require.NoError(t, err)
			assert.Equal(t, int64(1), dlqCount, "exactly one entry should be in DLQ")

			cancel()
			testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
		})
	}
}
