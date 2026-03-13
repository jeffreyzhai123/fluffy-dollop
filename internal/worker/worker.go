package worker

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/registry"
	"github.com/redis/go-redis/v9"
)

// Worker manages the lifecycle of the log processing worker
type Worker struct {
	consumer       queue.ConsumerInterface
	processor      ProcessorInterface
	config         *config.WorkerConfig
	logger         *observability.Logger
	sem            *semaphore.Weighted
	activeJobs     atomic.Int64
	backoff        *exponentialBackoff
	healthServer   HealthServerInterface
	ready          chan struct{}
	readyOnce      sync.Once
	workerID       string
	registry       *registry.Registry
	registryConfig *config.RegistryConfig
	redis          *redis.Client
	partitionID    int
}

type exponentialBackoff struct {
	current time.Duration
	min     time.Duration
	max     time.Duration
}

// newExponentialBackoff initializes a new exponential backoff with given parameters
func newExponentialBackoff(min, max time.Duration) *exponentialBackoff {
	return &exponentialBackoff{
		current: min,
		min:     min,
		max:     max,
	}
}

// NewWorker creates a new worker with all dependencies wired
func NewWorker(
	consumer queue.ConsumerInterface,
	processor ProcessorInterface,
	workerCfg *config.WorkerConfig,
	registryCfg *config.RegistryConfig,
	partitionID int,
	logger *observability.Logger,
	healthServer HealthServerInterface,
	workerID string,
	reg *registry.Registry,
	redisClient *redis.Client,
) *Worker {
	return &Worker{
		consumer:       consumer,
		processor:      processor,
		config:         workerCfg,
		registryConfig: registryCfg,
		partitionID:    partitionID,
		logger:         logger,
		sem:            semaphore.NewWeighted(int64(workerCfg.Concurrency)),
		backoff:        newExponentialBackoff(workerCfg.BackoffMinDelay, workerCfg.BackoffMaxDelay),
		healthServer:   healthServer,
		ready:          make(chan struct{}),
		workerID:       workerID,
		registry:       reg,
		redis:          redisClient,
	}
}

// Ready returns a channel that closes when worker is ready to process
func (w *Worker) Ready() <-chan struct{} {
	return w.ready
}

// Start begins the worker polling loop
// BLOCKING - runs until ctx is cancelled
func (w *Worker) Start(ctx context.Context) error {
	w.logger.Info("worker starting",
		"poll_interval", w.config.PollInterval,
		"concurrency", w.config.Concurrency,
		"reclaim_interval", w.config.ReclaimInterval,
		"reclaim_idle_time", w.config.ReclaimIdleTime,
	)

	// Starts main poll loop
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return w.pollLoop(ctx, g)
	})

	// Stale message reclaim loop
	g.Go(func() error {
		return w.reclaimLoop(ctx, g)
	})

	// Registry loop
	g.Go(func() error {
		return w.registryLoop(ctx)
	})

	// Start health server
	g.Go(func() error {
		return w.healthServer.Start(ctx)
	})

	w.logger.Info("worker started, polling for messages")
	err := g.Wait()
	w.logger.Info("waiting for in-flight jobs to complete")

	// Return worker error if any
	if err != nil && ctx.Err() == nil {
		w.logger.Error("worker stopped with error", "error", err)
		return err
	}

	w.logger.Info("all jobs completed, worker stopped")
	return nil
}

// pollLoop continuously polls Redis for messages until context is cancelled
func (w *Worker) pollLoop(ctx context.Context, g *errgroup.Group) error {
	// Signal ready when we start polling
	w.readyOnce.Do(func() {
		close(w.ready)
		w.logger.Debug("worker ready to process messages")
	})

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("poll loop exiting")
			return nil
		default:
			// Poll for next message
			envelope, messageID, err := w.consumer.Consume(ctx, w.config.PollInterval)
			if err != nil {
				w.logger.Error("error consuming from queue", "error", err)
				delay := w.backoff.Next()
				select {
				case <-time.After(delay):
					continue
				case <-ctx.Done(): // Interruptible sleep
					return nil
				}
			}

			w.backoff.Reset()

			// No message available (timeout) - loop again
			if envelope == nil {
				continue
			}

			// Acquire semaphore (blocks if at max concurrency)
			if err := w.sem.Acquire(ctx, 1); err != nil {
				return nil
			}

			g.Go(func() error {
				w.activeJobs.Add(1)
				defer func() {
					w.activeJobs.Add(-1)
					w.sem.Release(1)
				}()
				w.processMessage(ctx, envelope, messageID)
				return nil
			})
		}
	}
}

// reclaimLoop runs XAUTOCLAIM periodically but only on the worker that holds
// the reclaim lock. This prevents all workers from redundantly scanning the
// same PEL. The lock TTL equals the reclaim interval — if the lock holder dies,
// another worker picks it up on the next tick automatically.
func (w *Worker) reclaimLoop(ctx context.Context, g *errgroup.Group) error {
	ticker := time.NewTicker(w.config.ReclaimInterval)
	defer ticker.Stop()

	// Run immediately on start, then periodically
	if err := w.tryReclaim(ctx, g); err != nil {
		w.logger.Warn("initial reclaim failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("reclaim loop stopping, context cancelled")
			return nil
		case <-ticker.C:
			if err := w.tryReclaim(ctx, g); err != nil {
				w.logger.Warn("reclaim failed", "error", err)
				// Don't return - just log and continue
			}
		}
	}
}

// tryReclaim acquires the reclaim lock and runs reclaimStale if successful.
// Only one worker across the cluster runs reclaim at a time.
func (w *Worker) tryReclaim(ctx context.Context, g *errgroup.Group) error {
	lockKey := fmt.Sprintf("worker:reclaim:lock:%d", w.partitionID)
	acquired, err := w.redis.SetNX(ctx,
		lockKey,
		w.workerID,
		w.config.ReclaimInterval, // lock TTL = reclaim interval
	).Result()
	if err != nil {
		return fmt.Errorf("failed to acquire reclaim lock: %w", err)
	}
	if !acquired {
		return nil // another worker is handling reclaim this tick
	}

	return w.reclaimStale(ctx, g)
}

// reclaimStale reclaims stale messages via XAUTOCLAIM and processes them
// through the errgroup so they are drained on graceful shutdown.
func (w *Worker) reclaimStale(ctx context.Context, g *errgroup.Group) error {
	type reclaimableConsumer interface {
		ReclaimStale(ctx context.Context, idleTime time.Duration, maxBatch int) ([]*queue.JobEnvelope, []string, error)
	}

	consumer, ok := w.consumer.(reclaimableConsumer)
	if !ok {
		return nil
	}

	envelopes, messageIDs, err := consumer.ReclaimStale(ctx, w.config.ReclaimIdleTime, w.config.ReclaimBatchSize)
	if err != nil {
		return fmt.Errorf("failed to reclaim stale messages: %w", err)
	}

	for i, envelope := range envelopes {
		if err := w.sem.Acquire(ctx, 1); err != nil {
			return nil // context cancelled
		}
		e, id := envelope, messageIDs[i]
		// Use errgroup so reclaimed in-flight jobs are drained on shutdown
		g.Go(func() error {
			w.activeJobs.Add(1)
			defer func() {
				w.activeJobs.Add(-1)
				w.sem.Release(1)
			}()
			w.processMessage(ctx, e, id)
			return nil
		})
	}
	return nil
}

// registryLoop sends periodic heartbeats to the registry.
func (w *Worker) registryLoop(ctx context.Context) error {
	ticker := time.NewTicker(w.registryConfig.HeartbeatInterval)
	defer ticker.Stop()

	// Heartbeat immediately on start
	if err := w.registry.Heartbeat(ctx, w.workerID); err != nil {
		w.logger.Warn("initial heartbeat failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("registry loop stopping")
			return nil
		case <-ticker.C:
			if err := w.registry.Heartbeat(ctx, w.workerID); err != nil {
				w.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

// processMessage handles a single message: process then ack or retry on failure
func (w *Worker) processMessage(ctx context.Context, envelope *queue.JobEnvelope, messageID string) {

	w.logger.Info("processing job",
		"message_id", messageID,
		"active_jobs", w.activeJobs.Load(),
	)

	jobCtx, cancel := context.WithTimeout(context.Background(), w.config.ProcessingTimeout)
	defer cancel()

	// Attempt to process the message
	start := time.Now()
	err := w.processor.Process(jobCtx, envelope)
	duration := time.Since(start)

	if err != nil {
		w.logger.Error("job processing failed",
			"error", err,
			"message_id", messageID,
			"job_id", envelope.JobID,
			"duration", duration,
		)
		if retryErr := w.consumer.Retry(ctx, envelope, messageID, err); retryErr != nil {
			w.logger.Error("failed to retry message",
				"error", retryErr,
				"message_id", messageID,
			)
		}
		return
	}

	// Processing succeeded - ack the message
	if err := w.consumer.Ack(ctx, messageID); err != nil {
		w.logger.Error("failed to ack message",
			"error", err,
			"message_id", messageID,
		)
		return
	}

	w.logger.Info("message processed successfully",
		"message_id", messageID,
		"duration", time.Since(start),
	)
}

func (eb *exponentialBackoff) Next() time.Duration {
	current := eb.current
	eb.current = time.Duration(float64(eb.current) * 2)
	if eb.current > eb.max {
		eb.current = eb.max
	}
	// Add jitter (±25%) to prevent thundering herd
	jitter := time.Duration(float64(current) * 0.25 * (rand.Float64()*2 - 1))
	return current + jitter
}

func (eb *exponentialBackoff) Reset() {
	eb.current = eb.min
}

func (w *Worker) ActiveJobs() int64 {
	return w.activeJobs.Load()
}
