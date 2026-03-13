package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

type DefaultProcessor struct {
	logger *observability.Logger
}

func NewProcessor(logger *observability.Logger) *DefaultProcessor {
	return &DefaultProcessor{
		logger: logger,
	}
}

func (p *DefaultProcessor) Process(ctx context.Context, envelope *queue.JobEnvelope) error {
	p.logger.Info("processing log event",
		"job_id", envelope.JobID,
		"level", envelope.Data.Level,
		"service", envelope.Data.Service,
		"message", envelope.Data.Message,
		"retry_count", envelope.RetryCount,
	)

	//TEMP Simulate work that respects context cancellation
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case <-timer.C:
		p.logger.Info("job processing completed", "job_id", envelope.JobID)
		return nil
	case <-ctx.Done():
		p.logger.Warn("job processing cancelled", "job_id", envelope.JobID, "reason", ctx.Err())
		return fmt.Errorf("processing cancelled: %w", ctx.Err())
	}
}

// TODO: Validate log event
// TODO: Store in Postgres
// TODO: Update aggregations/metrics
