// internal/worker/interfaces.go
package worker

import (
	"context"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
)

// Processor defines the contract for processing jobs
type ProcessorInterface interface {
	Process(ctx context.Context, envelope *queue.JobEnvelope) error
}

// HealthServerInterface defines the contract for health servers
type HealthServerInterface interface {
	Start(ctx context.Context) error
}
