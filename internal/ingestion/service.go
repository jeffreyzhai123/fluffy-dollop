package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/queue"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
)

// Sentinel errors for error handling
var (
	ErrCircuitOpen      = errors.New("queue saturated, system overloaded")
	ErrValidationFailed = errors.New("log event validation failed")
)

type ErrRateLimited struct {
	RetryAfter time.Duration
}

func (e *ErrRateLimited) Error() string {
	return fmt.Sprintf("rate limited - retry after %v", e.RetryAfter)
}

type Service struct {
	producer  queue.ProducerInterface
	config    *config.IngestionConfig
	logger    *observability.Logger
	throttler ThrottlerInterface
}

func NewService(producer queue.ProducerInterface, cfg *config.Config, logger *observability.Logger, throttler ThrottlerInterface) *Service {
	return &Service{
		producer:  producer,
		config:    &cfg.Ingestion,
		logger:    logger,
		throttler: throttler,
	}
}

// IngestLog validates and enqueues a log event with circuit breaker protection
func (s *Service) IngestLog(ctx context.Context, logEvent *models.LogEvent) (string, error) {

	decision, retryAfter := s.throttler.Decision()

	switch decision {
	case ThrottleCircuitOpen:
		return "", ErrCircuitOpen
	case ThrottleRateLimit:
		return "", &ErrRateLimited{RetryAfter: retryAfter}
	case ThrottleAllow:
		// Continue to enqueue
	}

	if err := s.validateLogEvent(logEvent); err != nil {
		return "", ErrValidationFailed
	}

	// Enqueue to Redis
	messageID, err := s.producer.Enqueue(ctx, logEvent)
	if err != nil {
		s.logger.Error("failed to enqueue log", "error", err)
		return "", fmt.Errorf("enqueue failed: %w", err)
	}

	return messageID, nil
}

func (s *Service) validateLogEvent(logEvent *models.LogEvent) error {
	if logEvent == nil {
		return fmt.Errorf("log event is nil")
	}
	if logEvent.Level == "" {
		return fmt.Errorf("log level is required")
	}
	if logEvent.Message == "" {
		return fmt.Errorf("log message is required")
	}
	return nil
}
