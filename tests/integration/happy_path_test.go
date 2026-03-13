package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
)

func TestHappyPath(t *testing.T) {
	tests := []struct {
		name    string
		numJobs int
		timeout time.Duration
	}{
		{
			name:    "single job",
			numJobs: 1,
			timeout: 5 * time.Second,
		},
		{
			name:    "multiple jobs",
			numJobs: 5,
			timeout: 10 * time.Second,
		},
		{
			name:    "many jobs",
			numJobs: 20,
			timeout: 15 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSuite(t)

			// Enqueue jobs
			for i := 0; i < tt.numJobs; i++ {
				testLog := &models.LogEvent{
					Timestamp: time.Now(),
					Level:     "INFO",
					Service:   "test-service",
					Message:   "Test log message",
					Metadata: map[string]interface{}{
						"job_number": i,
						"test":       tt.name,
					},
				}

				messageID, err := s.Producer.Enqueue(context.Background(), testLog)
				require.NoError(t, err, "failed to enqueue job %d", i)

				if i == 0 {
					t.Logf("first message ID: %s", messageID)
				}
			}

			// Create context for worker
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Start worker
			processor := testutil.NewSucceedingProcessor(s.Logger)
			wrk := s.newWorker(t, ctx, processor)

			done := make(chan error, 1)
			go func() { done <- wrk.Start(ctx) }()

			// Wait for worker to be ready
			select {
			case <-wrk.Ready():
			case <-time.After(2 * time.Second):
				t.Fatal("worker did not become ready")
			}

			// Wait for ALL jobs to be processed
			testutil.WaitFor(t, tt.timeout, func() bool {
				return processor.ProcessCount() == int64(tt.numJobs)
			}, "all jobs to be processed")

			// Verify all jobs were processed
			assert.Equal(t, int64(tt.numJobs), processor.ProcessCount(),
				"should have processed all %d jobs", tt.numJobs)

			// Verify no pending messages
			testutil.WaitForPendingCount(t, s.Redis, s.Config.Queue.BaseStreamName,
				s.Config.Queue.BaseConsumerGroup, 0, 2*time.Second)

			pendingCount, err := testutil.GetPendingCount(s.Redis,
				s.Config.Queue.BaseStreamName, s.Config.Queue.BaseConsumerGroup)
			require.NoError(t, err)
			assert.Equal(t, int64(0), pendingCount, "should have no pending messages")

			// Stop worker gracefully
			cancel()

			select {
			case err := <-done:
				assert.NoError(t, err, "worker should shut down without error")
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not shut down")
			}

			t.Logf("processed %d jobs successfully", tt.numJobs)
		})
	}
}
