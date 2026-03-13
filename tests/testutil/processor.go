package testutil

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// ProcessorFunc is a function that processes a job envelope
type ProcessorFunc func(ctx context.Context, envelope *queue.JobEnvelope) error

// ControllableProcessor allows tests to inject custom behavior
type ControllableProcessor struct {
	processFunc  ProcessorFunc
	processCount atomic.Int64
	logger       *observability.Logger
}

// NewControllableProcessor creates a processor with custom behavior
func NewControllableProcessor(logger *observability.Logger, fn ProcessorFunc) *ControllableProcessor {
	return &ControllableProcessor{
		processFunc: fn,
		logger:      logger,
	}
}

// NewSucceedingProcessor creates a processor that always succeeds
func NewSucceedingProcessor(logger *observability.Logger) *ControllableProcessor {
	return NewControllableProcessor(logger, func(ctx context.Context, envelope *queue.JobEnvelope) error {
		// Simulate minimal work
		time.Sleep(10 * time.Millisecond)
		return nil
	})
}

// NewFailingProcessor creates a processor that always fails
func NewFailingProcessor(logger *observability.Logger) *ControllableProcessor {
	return NewControllableProcessor(logger, func(ctx context.Context, envelope *queue.JobEnvelope) error {
		return fmt.Errorf("simulated processing failure")
	})
}

// NewSlowProcessor creates a processor that sleeps for the given duration
// Use this to test timeouts
func NewSlowProcessor(logger *observability.Logger, delay time.Duration) *ControllableProcessor {
	return NewControllableProcessor(logger, func(ctx context.Context, envelope *queue.JobEnvelope) error {
		select {
		case <-time.After(delay):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

// NewConcurrencyTrackingProcessor tracks max concurrent executions
func NewConcurrencyTrackingProcessor(logger *observability.Logger, workDuration time.Duration) (*ControllableProcessor, *ConcurrencyTracker) {
	tracker := &ConcurrencyTracker{}

	processor := NewControllableProcessor(logger, func(ctx context.Context, envelope *queue.JobEnvelope) error {
		tracker.Enter()
		defer tracker.Exit()

		// Do some work
		select {
		case <-time.After(workDuration):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})

	return processor, tracker
}

// ConcurrencyTracker tracks concurrent executions
type ConcurrencyTracker struct {
	current atomic.Int64
	max     atomic.Int64
}

func (ct *ConcurrencyTracker) Enter() {
	current := ct.current.Add(1)

	// Update max if necessary (atomic CAS loop)
	for {
		oldMax := ct.max.Load()
		if current <= oldMax {
			break
		}
		if ct.max.CompareAndSwap(oldMax, current) {
			break
		}
	}
}

func (ct *ConcurrencyTracker) Exit() {
	ct.current.Add(-1)
}

func (ct *ConcurrencyTracker) Current() int64 {
	return ct.current.Load()
}

func (ct *ConcurrencyTracker) Max() int64 {
	return ct.max.Load()
}

// Process implements the worker.Processor interface
func (p *ControllableProcessor) Process(ctx context.Context, envelope *queue.JobEnvelope) error {
	p.processCount.Add(1)

	p.logger.Debug("test processor executing",
		"job_id", envelope.JobID,
		"retry_count", envelope.RetryCount,
		"process_count", p.processCount.Load(),
	)

	return p.processFunc(ctx, envelope)
}

// ProcessCount returns the total number of times Process was called
func (p *ControllableProcessor) ProcessCount() int64 {
	return p.processCount.Load()
}
