package worker

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/health"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// HealthServer is a simple HTTP server for worker health checks
type HealthServer struct {
	httpServer *http.Server
	logger     *observability.Logger
}

// NewHealthServer creates a health check server for the worker
func NewHealthServer(cfg *config.Config, healthAgg *health.Aggregator, logger *observability.Logger) *HealthServer {
	healthHandler := health.NewHTTPHandler(healthAgg, logger)

	mux := http.NewServeMux()
	health.RegisterRoutes(mux, healthHandler)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Worker.HealthPort,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return &HealthServer{
		httpServer: httpServer,
		logger:     logger,
	}
}

// Start starts the health server
func (s *HealthServer) Start(ctx context.Context) error {
	// Shutdown when context is cancelled
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("health server shutdown failed", "error", err)
		}
	}()

	s.logger.Info("health server starting", "addr", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("health server failed: %w", err)
	}
	return nil
}
