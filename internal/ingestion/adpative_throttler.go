package ingestion

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// ThrottlerMetrics holds cached queue health metrics
type ThrottlerMetrics struct {
	streamDepth      atomic.Int64
	pendingCount     atomic.Int64
	oldestMessageAge atomic.Int64 // seconds
	lastUpdated      atomic.Int64 // unix timestamp
}

// Throttler implements adaptive rate limiting based on queue health
type Throttler struct {
	producer queue.ProducerInterface
	consumer queue.ConsumerInterface
	config   *config.ThrottlerConfig
	logger   *observability.Logger
	metrics  *ThrottlerMetrics

	// Lifecycle management
	cancel   context.CancelFunc
	ctx      context.Context
	wg       sync.WaitGroup
	stopOnce sync.Once
}

// NewThrottler creates a new adaptive throttler
func NewThrottler(
	producer queue.ProducerInterface,
	consumer queue.ConsumerInterface,
	config *config.ThrottlerConfig,
	logger *observability.Logger,
) *Throttler {
	return &Throttler{
		producer: producer,
		consumer: consumer,
		config:   config,
		logger:   logger,
		metrics:  &ThrottlerMetrics{},
	}
}

// Start begins the background metrics updater
func (t *Throttler) Start(parentCtx context.Context) {
	if !t.config.Enabled {
		t.logger.Info("throttler disabled")
		return
	}

	t.ctx, t.cancel = context.WithCancel(parentCtx)

	t.wg.Add(1)
	go t.metricsUpdater()

	t.logger.Info("throttler started",
		"update_interval", t.config.MetricsUpdateInterval,
		"soft_depth", t.config.SoftDepthLimit,
		"hard_depth", t.config.HardDepthLimit,
		"rate_limit_score", t.config.RateLimitScore,
		"circuit_open_score", t.config.CircuitOpenScore,
		"estimated_drain_rate", t.config.EstimatedDrainRate,
	)
}

// Stop gracefully stops the throttler
func (t *Throttler) Stop() {
	t.stopOnce.Do(func() {
		if t.cancel != nil {
			t.cancel()
		}
		t.wg.Wait()
		t.logger.Info("throttler stopped")
	})
}

// metricsUpdater periodically fetches queue metrics
func (t *Throttler) metricsUpdater() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.MetricsUpdateInterval)
	defer ticker.Stop()

	// Update immediately on start
	t.updateMetrics()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.updateMetrics()
		}
	}
}

// updateMetrics fetches current queue health metrics
func (t *Throttler) updateMetrics() {
	// Use a timeout for metrics fetch
	fetchCtx, cancel := context.WithTimeout(t.ctx, 1*time.Second)
	defer cancel()

	// Get stream depth
	depth, err := t.producer.StreamLength(fetchCtx)
	if err != nil {
		t.logger.Warn("failed to get stream depth", "error", err)
		return
	}

	// Get pending count and oldest message age
	pending, oldestAge, err := t.getPendingMetrics(fetchCtx)
	if err != nil {
		t.logger.Warn("failed to get pending metrics", "error", err)
		pending = 0
		oldestAge = 0
	}

	// Update atomic metrics
	t.metrics.streamDepth.Store(depth)
	t.metrics.pendingCount.Store(pending)
	t.metrics.oldestMessageAge.Store(oldestAge)
	t.metrics.lastUpdated.Store(time.Now().Unix())

	t.logger.Debug("metrics updated",
		"depth", depth,
		"pending", pending,
		"oldest_age_sec", oldestAge,
	)
}

// getPendingMetrics gets pending message count and oldest message age
func (t *Throttler) getPendingMetrics(ctx context.Context) (int64, int64, error) {
	type pendingChecker interface {
		GetPendingInfo(ctx context.Context) (count int64, oldestAge int64, err error)
	}

	pc, ok := t.consumer.(pendingChecker)
	if !ok {
		return 0, 0, nil
	}

	return pc.GetPendingInfo(ctx)
}

// Decision determines if a request should be throttled
func (t *Throttler) Decision() (ThrottleDecision, time.Duration) {
	if !t.config.Enabled {
		return ThrottleAllow, 0
	}

	// Stale data protection - fail open
	lastUpdated := time.Unix(t.metrics.lastUpdated.Load(), 0)
	staleness := time.Since(lastUpdated)
	maxStaleness := 2 * t.config.MetricsUpdateInterval

	if staleness > maxStaleness {
		t.logger.Warn("metrics are stale, failing open",
			"staleness", staleness,
			"max_staleness", maxStaleness,
		)
		return ThrottleAllow, 0
	}

	depth := t.metrics.streamDepth.Load()
	oldestAge := time.Duration(t.metrics.oldestMessageAge.Load()) * time.Second

	// Calculate overload score
	depthRatio := float64(depth) / float64(t.config.SoftDepthLimit)
	ageRatio := float64(oldestAge) / float64(t.config.MaxMessageAge)
	overloadScore := max(depthRatio, ageRatio)

	// Circuit breaker - hard limit
	if overloadScore > t.config.CircuitOpenScore {
		t.logger.Warn("circuit breaker triggered",
			"overload_score", fmt.Sprintf("%.2f", overloadScore),
			"threshold", t.config.CircuitOpenScore,
			"depth", depth,
			"oldest_age", oldestAge,
		)
		return ThrottleCircuitOpen, 0
	}

	// Rate limiting - soft limit
	if overloadScore > t.config.RateLimitScore {
		retryAfter := t.computeRetryAfter(depth, oldestAge)
		t.logger.Info("rate limiting request",
			"overload_score", fmt.Sprintf("%.2f", overloadScore),
			"threshold", t.config.RateLimitScore,
			"retry_after", retryAfter,
		)
		return ThrottleRateLimit, retryAfter
	}

	// All good - allow
	return ThrottleAllow, 0
}

// computeRetryAfter calculates intelligent retry delay based on estimated drain rate
func (t *Throttler) computeRetryAfter(depth int64, oldestAge time.Duration) time.Duration {
	drainRate := t.config.EstimatedDrainRate

	// Fallback if no drain rate configured
	if drainRate <= 0 {
		return t.config.MinRetryAfter
	}

	// Calculate time to drain current backlog
	pending := t.metrics.pendingCount.Load()
	backlog := float64(depth + pending)
	retrySeconds := backlog / drainRate

	// Factor in message age - if messages are old, need more time
	if oldestAge > t.config.MaxMessageAge {
		ageMultiplier := float64(oldestAge) / float64(t.config.MaxMessageAge)
		retrySeconds *= ageMultiplier
	}

	retryAfter := time.Duration(retrySeconds) * time.Second

	// Clamp to min/max bounds
	if retryAfter < t.config.MinRetryAfter {
		retryAfter = t.config.MinRetryAfter
	}
	if retryAfter > t.config.MaxRetryAfter {
		retryAfter = t.config.MaxRetryAfter
	}

	return retryAfter
}

// GetMetrics returns current metrics snapshot (for debugging/monitoring)
func (t *Throttler) GetMetrics() (depth, pending, oldestAge int64) {
	return t.metrics.streamDepth.Load(),
		t.metrics.pendingCount.Load(),
		t.metrics.oldestMessageAge.Load()
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
