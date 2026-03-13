package ingestion

import (
	"context"
	"time"
)

// ThrottleDecision represents the throttling decision
type ThrottleDecision int

const (
	ThrottleAllow ThrottleDecision = iota
	ThrottleRateLimit
	ThrottleCircuitOpen
)

// ThrottlerInterface defines the contract for request throttling
type ThrottlerInterface interface {
	Start(ctx context.Context)
	Stop()
	Decision() (ThrottleDecision, time.Duration)
	GetMetrics() (depth, pending, oldestAge int64)
}

// NoopThrottler always allows requests (for testing or when disabled)
type NoopThrottler struct{}

func (n *NoopThrottler) Start(ctx context.Context) {}

func (n *NoopThrottler) Stop() {}

func (n *NoopThrottler) Decision() (ThrottleDecision, time.Duration) {
	return ThrottleAllow, 0
}

func (n *NoopThrottler) GetMetrics() (depth, pending, oldestAge int64) {
	return 0, 0, 0
}

// Ensure implementation
var _ ThrottlerInterface = (*Throttler)(nil)
var _ ThrottlerInterface = (*NoopThrottler)(nil)
