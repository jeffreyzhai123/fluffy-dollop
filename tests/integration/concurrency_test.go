package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorker_GracefulShutdown verifies that cancelling the worker context:
//  1. Allows in-flight jobs to complete (not dropped mid-processing)
//  2. Returns from Start() without error
//  3. Drains the semaphore (ActiveJobs reaches zero)
func TestWorker_GracefulShutdown(t *testing.T) {
	tests := []struct {
		name          string
		numJobs       int
		jobDuration   time.Duration
		shutdownDelay time.Duration // how long after ready() before we cancel
	}{
		{
			name:          "shutdown with no active jobs",
			numJobs:       0,
			jobDuration:   10 * time.Millisecond,
			shutdownDelay: 100 * time.Millisecond,
		},
		{
			name:          "shutdown while jobs are in-flight",
			numJobs:       3,
			jobDuration:   300 * time.Millisecond,
			shutdownDelay: 50 * time.Millisecond, // cancel before jobs finish
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSuite(t)

			for i := 0; i < tt.numJobs; i++ {
				_, err := s.Producer.Enqueue(context.Background(), &models.LogEvent{
					Timestamp: time.Now(),
					Level:     "INFO",
					Service:   "shutdown-test",
					Message:   fmt.Sprintf("job %d", i),
				})
				require.NoError(t, err)
			}

			ctx, cancel := context.WithCancel(context.Background())

			processor := testutil.NewSlowProcessor(s.Logger, tt.jobDuration)
			wrk := s.newWorker(t, ctx, processor)

			done := make(chan error, 1)
			go func() { done <- wrk.Start(ctx) }()

			select {
			case <-wrk.Ready():
			case <-time.After(2 * time.Second):
				t.Fatal("worker did not become ready")
			}

			// Give the worker time to pick up jobs before cancelling
			time.Sleep(tt.shutdownDelay)
			cancel()

			err := testutil.WaitForWorkerShutdown(t, done, 10*time.Second)
			assert.NoError(t, err, "Start() should return nil on clean shutdown")

			// After shutdown no jobs should be actively running
			assert.Equal(t, int64(0), wrk.ActiveJobs(),
				"ActiveJobs should be zero after shutdown")
		})
	}
}

// TestWorker_ConcurrencyLimit verifies the semaphore enforces the configured
// MaxConcurrency — no more than N jobs should run simultaneously.
func TestWorker_ConcurrencyLimit(t *testing.T) {
	tests := []struct {
		name           string
		maxConcurrency int
		numJobs        int
	}{
		{
			name:           "limit of 2",
			maxConcurrency: 2,
			numJobs:        10,
		},
		{
			name:           "limit of 5",
			maxConcurrency: 5,
			numJobs:        20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSuite(t)
			s.Config.Worker.Concurrency = tt.maxConcurrency

			for i := 0; i < tt.numJobs; i++ {
				_, err := s.Producer.Enqueue(context.Background(), &models.LogEvent{
					Timestamp: time.Now(),
					Level:     "DEBUG",
					Service:   "concurrency-test",
					Message:   fmt.Sprintf("job %d", i),
				})
				require.NoError(t, err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// workDuration long enough that multiple jobs overlap
			processor, tracker := testutil.NewConcurrencyTrackingProcessor(
				s.Logger, 150*time.Millisecond,
			)

			wrk := s.newWorker(t, ctx, processor)
			done := make(chan error, 1)
			go func() { done <- wrk.Start(ctx) }()

			select {
			case <-wrk.Ready():
			case <-time.After(2 * time.Second):
				t.Fatal("worker did not become ready")
			}

			// Wait for all jobs to complete
			testutil.WaitFor(t, 30*time.Second, func() bool {
				return processor.ProcessCount() == int64(tt.numJobs)
			}, fmt.Sprintf("all %d jobs to complete", tt.numJobs))

			assert.LessOrEqual(t, tracker.Max(), int64(tt.maxConcurrency),
				"max concurrent executions must not exceed MaxConcurrency")

			// Sanity: with enough jobs we should have actually hit the limit
			if tt.numJobs >= tt.maxConcurrency*2 {
				assert.Equal(t, int64(tt.maxConcurrency), tracker.Max(),
					"should have saturated the concurrency limit")
			}

			cancel()
			testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
		})
	}
}

// TestWorker_ReclaimStaleMessages verifies the reclaimLoop via XAUTOCLAIM:
// messages that were delivered but never ACKed (simulating a crashed consumer)
// are picked up and processed by the reclaim path.
func TestWorker_ReclaimStaleMessages(t *testing.T) {
	s := newSuite(t)
	s.Config.Worker.ReclaimIdleTime = 2 * time.Second
	s.Config.Worker.ReclaimInterval = 1 * time.Second
	s.Config.Worker.PollInterval = 500 * time.Millisecond // unblock quickly to check pending

	ctx := context.Background()

	const numJobs = 3
	for i := 0; i < numJobs; i++ {
		_, err := s.Producer.Enqueue(ctx, &models.LogEvent{
			Timestamp: time.Now(),
			Level:     "INFO",
			Service:   "reclaim-test",
			Message:   fmt.Sprintf("stale job %d", i),
		})
		require.NoError(t, err)
	}

	// --- Phase 1: deliver messages to abandoned consumer, never ack ---
	// Create group first, then read with raw Redis client so we bypass
	// the pending-drain logic in Consume and deliver all 3 distinct messages.
	err := s.Redis.XGroupCreateMkStream(ctx,
		s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup, "0").Err()
	require.NoError(t, err, "failed to create consumer group")

	const abandonedConsumer = "abandoned-consumer-will-never-ack"
	for i := 0; i < numJobs; i++ {
		streams, err := s.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    s.Config.Queue.BaseConsumerGroup,
			Consumer: abandonedConsumer,
			Streams:  []string{s.Config.Queue.BaseStreamName, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()
		require.NoError(t, err, "failed to read message %d", i)
		require.Len(t, streams[0].Messages, 1, "expected one message")
	}

	// Verify all messages are pending under the abandoned consumer
	pendingBefore, err := testutil.GetPendingCount(s.Redis,
		s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup)
	require.NoError(t, err)
	require.Equal(t, int64(numJobs), pendingBefore, "all messages should be pending")

	// --- Phase 2: wait for messages to become stale ---
	time.Sleep(s.Config.Worker.ReclaimIdleTime + 1*time.Second)

	// --- Phase 3: start worker — reclaimLoop claims into PEL,
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

	testutil.WaitFor(t, 15*time.Second, func() bool {
		return processor.ProcessCount() == int64(numJobs)
	}, fmt.Sprintf("reclaim loop to process all %d stale messages", numJobs))

	assert.Equal(t, int64(numJobs), processor.ProcessCount())

	testutil.WaitForPendingCount(t, s.Redis,
		s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup,
		0, 5*time.Second)

	cancel()
	testutil.WaitForWorkerShutdown(t, done, 5*time.Second)
}
