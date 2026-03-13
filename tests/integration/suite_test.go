package integration

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/bootstrap"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/registry"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/worker"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
)

// Suite holds shared test dependencies
type Suite struct {
	Redis    *redis.Client
	Producer queue.ProducerInterface
	DLQ      *queue.DLQ
	Config   *config.Config
	Logger   *observability.Logger
	Registry *registry.Registry
}

func TestMain(m *testing.M) {
	if os.Getenv("INTEGRATION_TESTS") != "true" {
		fmt.Println("Skipping integration tests (set INTEGRATION_TESTS=true)")
		os.Exit(0)
	}

	if err := testutil.LoadTestEnv(); err != nil {
		fmt.Printf("ERROR: %v\n", err)
		fmt.Println("Make sure .env.test exists in project root")
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// newSuite creates a fresh test suite using production setup code
func newSuite(t *testing.T) *Suite {
	t.Helper()

	// Test-specific config
	cfg, err := config.Load()
	require.NoError(t, err, "failed to load config")

	// Override with unique names per test
	cfg.Queue.BaseStreamName = testutil.UniqueName(t, "stream")
	cfg.Queue.BaseConsumerGroup = testutil.UniqueName(t, "group")
	cfg.Queue.DLQStreamName = testutil.UniqueName(t, "dlq")

	logger := observability.NewLogger(&cfg.Observability, "test")

	// Setup Redis with context
	redisClient, err := bootstrap.SetupRedis(context.Background(), cfg, logger)
	require.NoError(t, err, "failed to connect to test Redis at localhost:6380")

	// Create partition streams and consumer groups before anything else
	err = bootstrap.SetupPartitions(context.Background(), redisClient, cfg, logger)
	require.NoError(t, err, "failed to setup partitions")

	// Clean up after test
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		// Delete the stream and consumer group with it (best effort)
		for i := 0; i < cfg.Queue.PartitionCount; i++ {
			streamName := bootstrap.PartitionStreamName(cfg.Queue.BaseStreamName, i)
			if err := redisClient.Del(cleanupCtx, streamName).Err(); err != nil {
				t.Logf("warning: failed to delete partition stream %d: %v", i, err)
			}
		}
		// Delete the DLQ stream (best effort)
		if err := redisClient.Del(cleanupCtx, cfg.Queue.DLQStreamName).Err(); err != nil {
			t.Logf("warning: failed to delete DLQ stream during cleanup: %v", err)
		}
		redisClient.Close()
	})

	// Use production producer setup
	producer := bootstrap.SetupProducer(redisClient, cfg, logger)
	dlq := bootstrap.SetupDLQ(redisClient, cfg, logger)
	reg := bootstrap.SetupRegistry(redisClient, cfg, logger)

	return &Suite{
		Redis:    redisClient,
		Producer: producer,
		DLQ:      dlq,
		Config:   cfg,
		Logger:   logger,
		Registry: reg,
	}
}

// newWorker creates a worker for testing.
// Tests don't use partitioning so partitionEpochs is empty — checkEpoch
// is a no-op until partition-aware consumers are wired in.
func (s *Suite) newWorker(t *testing.T, ctx context.Context, processor worker.ProcessorInterface) *worker.Worker {
	t.Helper()

	workerID := testutil.UniqueName(t, "worker")
	consumer, err := bootstrap.SetupConsumer(ctx, s.Redis, s.DLQ, s.Config, s.Logger, workerID)
	require.NoError(t, err, "failed to setup consumer")

	return worker.NewWorker(consumer, processor, &s.Config.Worker, &s.Config.Registry, 0, s.Logger, testutil.NewNoopHealthServer(), workerID, s.Registry, s.Redis)
}

// setupRawConsumer creates a consumer with a distinct identity for reclaim tests
func (s *Suite) setupRawConsumer(t *testing.T, ctx context.Context) (queue.ConsumerInterface, error) {
	t.Helper()
	consumerID := testutil.UniqueName(t, "abandoned-consumer")
	return bootstrap.SetupConsumer(ctx, s.Redis, s.DLQ, s.Config, s.Logger, consumerID)
}
