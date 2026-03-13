package registry

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

const controlPlaneLockKey = "controlplane:lock"

// ControlPlane detects dead workers and logs alerts.
// It does not assign or reassign partitions — Redis consumer groups handle
// message failover automatically. The control plane exists for operational
// visibility: detecting prolonged outages before they breach SLAs.
type ControlPlane struct {
	registry *Registry
	config   *config.RegistryConfig
	logger   *observability.Logger
	id       string
}

func NewControlPlane(
	registry *Registry,
	cfg *config.RegistryConfig,
	logger *observability.Logger,
) *ControlPlane {
	hostname, _ := os.Hostname()
	id := fmt.Sprintf("controlplane-%s-%d", hostname, os.Getpid())

	return &ControlPlane{
		registry: registry,
		config:   cfg,
		logger:   logger,
		id:       id,
	}
}

// Start blocks until ctx is cancelled. Acquires the control plane lock then
// runs the reconcile loop at ControlPlanePoll interval.
func (cp *ControlPlane) Start(ctx context.Context) error {
	cp.logger.Info("control plane starting", "id", cp.id)

	acquired, err := cp.acquireLock(ctx)
	if err != nil {
		return fmt.Errorf("failed to acquire control plane lock: %w", err)
	}
	if !acquired {
		cp.logger.Warn("another control plane instance is running, standing by", "id", cp.id)
		return cp.waitForLock(ctx)
	}

	cp.logger.Info("control plane lock acquired", "id", cp.id)

	reconcileTicker := time.NewTicker(cp.config.ControlPlanePoll)
	lockRenewTicker := time.NewTicker(cp.config.LockTTL / 2)
	defer reconcileTicker.Stop()
	defer lockRenewTicker.Stop()

	// Run immediately on start
	cp.reconcile(ctx)

	for {
		select {
		case <-ctx.Done():
			cp.logger.Info("control plane stopping")
			cp.releaseLock(context.Background())
			return nil

		case <-lockRenewTicker.C:
			if err := cp.renewLock(ctx); err != nil {
				cp.logger.Error("failed to renew control plane lock", "error", err)
				return fmt.Errorf("lost control plane lock: %w", err)
			}

		case <-reconcileTicker.C:
			cp.reconcile(ctx)
		}
	}
}

// reconcile checks all registered workers for liveness and logs dead workers.
// Redis consumer groups handle message failover — this is purely observational.
func (cp *ControlPlane) reconcile(ctx context.Context) {
	workers, err := cp.registry.ListWorkers(ctx)
	if err != nil {
		cp.logger.Warn("reconcile: failed to list workers", "error", err)
		return
	}

	aliveCount := 0
	for _, w := range workers {
		if w.IsAlive(cp.config.DeadThreshold) {
			aliveCount++
			continue
		}

		// Dead worker — log alert. Redis consumer groups will reclaim
		// its PEL messages automatically via XAUTOCLAIM in alive workers.
		cp.logger.Error("dead worker detected",
			"worker_id", w.WorkerID,
			"last_seen", w.LastSeen,
			"seconds_since_heartbeat", time.Since(w.LastSeen).Seconds(),
		)
	}

	cp.logger.Info("reconcile complete",
		"total_workers", len(workers),
		"alive", aliveCount,
		"dead", len(workers)-aliveCount,
	)
}

func (cp *ControlPlane) acquireLock(ctx context.Context) (bool, error) {
	ok, err := cp.registry.client.SetNX(ctx,
		controlPlaneLockKey,
		cp.id,
		cp.config.LockTTL,
	).Result()
	if err != nil {
		return false, fmt.Errorf("lock SetNX failed: %w", err)
	}
	return ok, nil
}

func (cp *ControlPlane) renewLock(ctx context.Context) error {
	current, err := cp.registry.client.Get(ctx, controlPlaneLockKey).Result()
	if err != nil {
		return fmt.Errorf("failed to read lock: %w", err)
	}
	if current != cp.id {
		return fmt.Errorf("lock owned by %q, not %q", current, cp.id)
	}
	if err := cp.registry.client.Expire(ctx, controlPlaneLockKey, cp.config.LockTTL).Err(); err != nil {
		return fmt.Errorf("failed to renew lock TTL: %w", err)
	}
	return nil
}

func (cp *ControlPlane) releaseLock(ctx context.Context) {
	current, err := cp.registry.client.Get(ctx, controlPlaneLockKey).Result()
	if err != nil || current != cp.id {
		return
	}
	cp.registry.client.Del(ctx, controlPlaneLockKey)
	cp.logger.Info("control plane lock released", "id", cp.id)
}

func (cp *ControlPlane) waitForLock(ctx context.Context) error {
	ticker := time.NewTicker(cp.config.LockTTL)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			acquired, err := cp.acquireLock(ctx)
			if err != nil {
				cp.logger.Warn("failed to acquire lock while waiting", "error", err)
				continue
			}
			if acquired {
				cp.logger.Info("control plane lock acquired after waiting", "id", cp.id)
				return cp.Start(ctx)
			}
		}
	}
}
