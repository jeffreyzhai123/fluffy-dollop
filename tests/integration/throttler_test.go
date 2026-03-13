package integration

import (
	"context"
	"testing"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/ingestion"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
	"github.com/jeffreyzhai123/fluffy-dollop/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// throttlerTestConfig returns a ThrottlerConfig tuned for fast integration tests.
//
// Thresholds at these settings:
//
//	RateLimit  trips when depth > SoftDepthLimit(5) * RateLimitScore(1.0)   → depth >= 6
//	Circuit    trips when depth > SoftDepthLimit(5) * CircuitOpenScore(1.5)  → depth >= 8
func throttlerTestConfig() config.ThrottlerConfig {
	return config.ThrottlerConfig{
		Enabled:               true,
		SoftDepthLimit:        5,
		HardDepthLimit:        20,
		RateLimitScore:        1.0,
		CircuitOpenScore:      1.5,
		MaxMessageAge:         30 * time.Second,
		CriticalMessageAge:    60 * time.Second,
		MetricsUpdateInterval: 100 * time.Millisecond,
		EstimatedDrainRate:    10,
		MinRetryAfter:         1 * time.Second,
		MaxRetryAfter:         30 * time.Second,
	}
}

// setupThrottler constructs and starts a real Throttler backed by the suite's
// Redis. The throttler is stopped via t.Cleanup so goroutines never leak.
// Returns the concrete *ingestion.Throttler so tests can poll Decision() directly.
func setupThrottler(t *testing.T, s *Suite, ctx context.Context) *ingestion.Throttler {
	t.Helper()

	consumer, err := s.setupRawConsumer(t, ctx)
	require.NoError(t, err, "failed to create throttler consumer")

	throttler := ingestion.NewThrottler(
		s.Producer,
		consumer,
		&s.Config.Ingestion.Throttler,
		s.Logger,
	)
	throttler.Start(ctx)
	t.Cleanup(func() { throttler.Stop() })

	return throttler
}

// TestThrottler_CircuitBreaker verifies that once queue depth exceeds
// SoftDepthLimit * CircuitOpenScore the service returns ErrCircuitOpen.
//
// With SoftDepthLimit=5 and CircuitOpenScore=1.5 the circuit trips at depth > 7.5.
// We enqueue 10 messages to ensure we are clearly above the threshold.
func TestThrottler_CircuitBreaker(t *testing.T) {
	s := newSuite(t)
	s.Config.Ingestion.Throttler = throttlerTestConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	throttler := setupThrottler(t, s, ctx)
	svc := ingestion.NewService(s.Producer, s.Config, s.Logger, throttler)

	// Enqueue enough to breach the circuit threshold (need depth > 7.5, use 10)
	const depthNeeded = 10
	for i := 0; i < depthNeeded; i++ {
		_, err := s.Producer.Enqueue(ctx, &models.LogEvent{
			Timestamp: time.Now(),
			Level:     "INFO",
			Service:   "circuit-test",
			Message:   "filling the queue",
		})
		require.NoError(t, err, "failed to enqueue message %d", i)
	}

	// Poll Decision() directly — waits on the actual state change rather than
	// guessing how long the metrics update cycle takes.
	testutil.WaitFor(t, 5*time.Second, func() bool {
		decision, _ := throttler.Decision()
		return decision == ingestion.ThrottleCircuitOpen
	}, "throttler to trip circuit breaker")

	// Service must reject new ingestion with ErrCircuitOpen.
	_, err := svc.IngestLog(ctx, &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "circuit-test",
		Message:   "this should be rejected",
	})
	assert.ErrorIs(t, err, ingestion.ErrCircuitOpen,
		"IngestLog should return ErrCircuitOpen when circuit is open")

	// Must be a hard rejection — not a 429 misfire.
	var rateLimitErr *ingestion.ErrRateLimited
	assert.NotErrorAs(t, err, &rateLimitErr,
		"error should be ErrCircuitOpen, not ErrRateLimited")
}

// TestThrottler_RateLimit verifies that queue depth in the soft zone
// (score > RateLimitScore but <= CircuitOpenScore) produces ErrRateLimited
// with a RetryAfter duration clamped within configured bounds.
//
// Depth of 6 gives score = 6/5 = 1.2 → above RateLimitScore(1.0), below CircuitOpenScore(1.5).
func TestThrottler_RateLimit(t *testing.T) {
	s := newSuite(t)
	cfg := throttlerTestConfig()
	s.Config.Ingestion.Throttler = cfg

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	throttler := setupThrottler(t, s, ctx)
	svc := ingestion.NewService(s.Producer, s.Config, s.Logger, throttler)

	// Depth of 6: score = 1.2 — in the rate-limit zone, not circuit-open zone.
	const depthNeeded = 6
	for i := 0; i < depthNeeded; i++ {
		_, err := s.Producer.Enqueue(ctx, &models.LogEvent{
			Timestamp: time.Now(),
			Level:     "WARN",
			Service:   "ratelimit-test",
			Message:   "filling to rate limit zone",
		})
		require.NoError(t, err, "failed to enqueue message %d", i)
	}

	// Wait for throttler to observe the depth and enter rate-limit state.
	testutil.WaitFor(t, 5*time.Second, func() bool {
		decision, _ := throttler.Decision()
		return decision == ingestion.ThrottleRateLimit
	}, "throttler to enter rate-limit state")

	// Belt-and-suspenders: confirm we landed in the soft zone, not circuit-open.
	decision, _ := throttler.Decision()
	assert.Equal(t, ingestion.ThrottleRateLimit, decision,
		"should be rate-limited, not circuit-open")

	_, err := svc.IngestLog(ctx, &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "ratelimit-test",
		Message:   "this should be rate limited",
	})
	require.Error(t, err)

	var rateLimitErr *ingestion.ErrRateLimited
	assert.ErrorAs(t, err, &rateLimitErr,
		"IngestLog should return ErrRateLimited in soft overload zone")
	assert.Greater(t, rateLimitErr.RetryAfter, time.Duration(0),
		"RetryAfter should be positive")
	assert.GreaterOrEqual(t, rateLimitErr.RetryAfter, cfg.MinRetryAfter,
		"RetryAfter should not be below MinRetryAfter")
	assert.LessOrEqual(t, rateLimitErr.RetryAfter, cfg.MaxRetryAfter,
		"RetryAfter should not exceed MaxRetryAfter")
}

// TestThrottler_StaleMetricsFallsOpen verifies the fail-open safety behaviour:
// when the metrics update loop stops and cached data ages beyond 2*MetricsUpdateInterval,
// Decision() must return ThrottleAllow even if cached depth would circuit-break.
//
// The test loads the queue past the circuit threshold BEFORE stopping the loop,
// proving that without fail-open logic the assertion would fail — we are testing
// a real safety property, not an idle-queue trivial case.
func TestThrottler_StaleMetricsFailsOpen(t *testing.T) {
	s := newSuite(t)
	cfg := throttlerTestConfig()
	cfg.MetricsUpdateInterval = 100 * time.Millisecond // stale window = 200ms
	s.Config.Ingestion.Throttler = cfg

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	throttler := setupThrottler(t, s, ctx)
	svc := ingestion.NewService(s.Producer, s.Config, s.Logger, throttler)

	// Load the queue well past the circuit threshold.
	const depthNeeded = 10
	for i := 0; i < depthNeeded; i++ {
		_, err := s.Producer.Enqueue(ctx, &models.LogEvent{
			Timestamp: time.Now(),
			Level:     "WARN",
			Service:   "stale-test",
			Message:   "pre-loading queue",
		})
		require.NoError(t, err)
	}

	// Confirm circuit is open while metrics are fresh.
	testutil.WaitFor(t, 5*time.Second, func() bool {
		decision, _ := throttler.Decision()
		return decision == ingestion.ThrottleCircuitOpen
	}, "throttler to reach CircuitOpen before stopping update loop")

	// Idempotent stop
	throttler.Stop()

	// Wait for the stale window to expire: 2*100ms + 200ms buffer = 400ms.
	time.Sleep(2*cfg.MetricsUpdateInterval + 200*time.Millisecond)

	// Despite cached depth still being 10, Decision() must fail open.
	decision, _ := throttler.Decision()
	assert.Equal(t, ingestion.ThrottleAllow, decision,
		"throttler should fail open when metrics are stale")

	// End-to-end: the service must accept requests in fail-open state.
	_, err := svc.IngestLog(context.Background(), &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "stale-test",
		Message:   "should be allowed through on stale metrics",
	})
	assert.NoError(t, err,
		"IngestLog should succeed when throttler is in fail-open state")
}
