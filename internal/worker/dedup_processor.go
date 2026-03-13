package worker

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/redis/go-redis/v9"
)

type DeduplicatingProcessor struct {
	downstream     ProcessorInterface
	redis          *redis.Client
	dedupTTL       time.Duration
	logger         *observability.Logger
	duplicateCount atomic.Int64
}

func NewDeduplicatingProcessor(
	downstream ProcessorInterface,
	redis *redis.Client,
	dedupTTL time.Duration,
	logger *observability.Logger,
) *DeduplicatingProcessor {
	return &DeduplicatingProcessor{
		downstream: downstream,
		redis:      redis,
		dedupTTL:   dedupTTL,
		logger:     logger,
	}
}

func (p *DeduplicatingProcessor) Process(ctx context.Context, envelope *queue.JobEnvelope) error {
	// Allows for retries, while avoiding reprocessing (crash before ack) of already succeeded jobs with the same JobID.
	key := fmt.Sprintf("dedup:%s:%d", envelope.JobID, envelope.RetryCount)

	// SetNX — only sets if key doesn't exist
	wasNew, err := p.redis.SetNX(ctx, key, "1", p.dedupTTL).Result()
	if err != nil {
		return fmt.Errorf("dedup check failed: %w", err)
	}
	if !wasNew {
		p.logger.Info("duplicate job skipped", "job_id", envelope.JobID)
		p.duplicateCount.Add(1)
		return nil // ack and move on
	}

	// First time seeing this job — process it
	return p.downstream.Process(ctx, envelope)
}

func (p *DeduplicatingProcessor) DuplicateCount() int64 {
	return p.duplicateCount.Load()
}
