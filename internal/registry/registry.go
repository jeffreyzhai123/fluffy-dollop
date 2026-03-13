package registry

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/redis/go-redis/v9"
)

const (
	workerKeyPrefix = "worker:registry:"

	fieldLastSeen  = "last_seen"
	fieldStartedAt = "started_at"
	fieldWorkerID  = "worker_id"
)

// Registry is a Redis-backed observability layer for worker liveness.
// It does not coordinate partitions or assignments — Redis consumer groups
// handle message distribution. The registry exists so the control plane
// can detect dead workers and alert, and so operators can query worker state.
type Registry struct {
	client *redis.Client
	config *config.RegistryConfig
	logger *observability.Logger
}

func NewRegistry(
	client *redis.Client,
	cfg *config.RegistryConfig,
	logger *observability.Logger,
) *Registry {
	return &Registry{
		client: client,
		config: cfg,
		logger: logger,
	}
}

// Register writes the worker's initial record to Redis.
// Safe to call on restart — overwrites any stale record for the same workerID.
func (r *Registry) Register(ctx context.Context, workerID string) error {
	now := time.Now().Unix()

	err := r.client.HSet(ctx, workerKey(workerID), map[string]interface{}{
		fieldWorkerID:  workerID,
		fieldLastSeen:  now,
		fieldStartedAt: now,
	}).Err()
	if err != nil {
		return fmt.Errorf("failed to register worker %s: %w", workerID, err)
	}

	r.logger.Info("worker registered", "worker_id", workerID)
	return nil
}

// Heartbeat updates last_seen for the worker. Called periodically by the worker.
func (r *Registry) Heartbeat(ctx context.Context, workerID string) error {
	err := r.client.HSet(ctx, workerKey(workerID),
		fieldLastSeen, time.Now().Unix(),
	).Err()
	if err != nil {
		return fmt.Errorf("failed to heartbeat worker %s: %w", workerID, err)
	}
	return nil
}

// Deregister removes the worker record on graceful shutdown.
func (r *Registry) Deregister(ctx context.Context, workerID string) error {
	if err := r.client.Del(ctx, workerKey(workerID)).Err(); err != nil {
		return fmt.Errorf("failed to deregister worker %s: %w", workerID, err)
	}
	r.logger.Info("worker deregistered", "worker_id", workerID)
	return nil
}

// GetWorker returns the WorkerInfo for the given workerID.
// Returns nil, nil if the worker does not exist.
func (r *Registry) GetWorker(ctx context.Context, workerID string) (*WorkerInfo, error) {
	fields, err := r.client.HGetAll(ctx, workerKey(workerID)).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get worker %s: %w", workerID, err)
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return parseWorkerInfo(fields)
}

// ListWorkers returns all registered workers.
func (r *Registry) ListWorkers(ctx context.Context) ([]*WorkerInfo, error) {
	keys, err := r.client.Keys(ctx, workerKeyPrefix+"*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to list worker keys: %w", err)
	}

	workers := make([]*WorkerInfo, 0, len(keys))
	for _, key := range keys {
		fields, err := r.client.HGetAll(ctx, key).Result()
		if err != nil {
			r.logger.Warn("failed to get worker fields", "key", key, "error", err)
			continue
		}
		if len(fields) == 0 {
			continue
		}
		info, err := parseWorkerInfo(fields)
		if err != nil {
			r.logger.Warn("failed to parse worker info", "key", key, "error", err)
			continue
		}
		workers = append(workers, info)
	}
	return workers, nil
}

func workerKey(workerID string) string {
	return workerKeyPrefix + workerID
}

func parseWorkerInfo(fields map[string]string) (*WorkerInfo, error) {
	workerID := fields[fieldWorkerID]
	if workerID == "" {
		return nil, fmt.Errorf("missing required field %q", fieldWorkerID)
	}

	lastSeenUnix, err := strconv.ParseInt(fields[fieldLastSeen], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid last_seen %q: %w", fields[fieldLastSeen], err)
	}
	startedAtUnix, err := strconv.ParseInt(fields[fieldStartedAt], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid started_at %q: %w", fields[fieldStartedAt], err)
	}

	return &WorkerInfo{
		WorkerID:  workerID,
		LastSeen:  time.Unix(lastSeenUnix, 0),
		StartedAt: time.Unix(startedAtUnix, 0),
	}, nil
}
