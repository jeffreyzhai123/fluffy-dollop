package ingestion

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/health"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// Server manages the HTTP server lifecycle for the ingestion service
type Server struct {
	httpServer *http.Server
	config     *config.Config
	logger     *observability.Logger
	healthAgg  *health.Aggregator
}

// NewServer creates a new ingestion server with all dependencies wired
func NewServer(cfg *config.Config, logger *observability.Logger, healthAgg *health.Aggregator, service *Service) *Server {

	handler := NewHandler(logger, service)
	healthHandler := health.NewHTTPHandler(healthAgg, logger)

	router := NewRouter(handler, healthHandler, logger)

	httpServer := buildHTTPServer(cfg, router, logger)

	return &Server{
		httpServer: httpServer,
		config:     cfg,
		logger:     logger,
		healthAgg:  healthAgg,
	}
}

// buildHTTPServer creates and configures the http.Server with timeouts
func buildHTTPServer(cfg *config.Config, handler http.Handler, logger *observability.Logger) *http.Server {
	return &http.Server{
		Addr:         cfg.ServerAddr(),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
		// ErrorLog: adapt stdlib logger to use our structured logger
		ErrorLog: log.New(&errorLogWriter{logger: logger}, "", 0),
	}
}

// Start begins listening for HTTP requests (blocking call, runs until server is shut down)
func (s *Server) Start(ctx context.Context) error {
	s.logger.Info("starting ingestion server",
		"address", s.httpServer.Addr,
		"service", "ingestion-service",
	)

	serverErr := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for shutdown signal or server error
	select {
	case <-ctx.Done():
		s.logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			s.logger.Error("server failed to start", "error", err)
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.Server.ShutdownTimeout)
	defer cancel()

	if err := s.Shutdown(shutdownCtx); err != nil {
		s.logger.Error("graceful shutdown failed, forcing close",
			"error", err,
			"is_timeout", errors.Is(err, context.DeadlineExceeded),
		)

		// Force close as fallback
		if closeErr := s.httpServer.Close(); closeErr != nil {
			s.logger.Error("force close failed", "error", closeErr)
		}

		return fmt.Errorf("shutdown failed: %w", err)
	}

	s.logger.Info("server shutdown completed successfully")
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("initiating graceful shutdown",
		"timeout", s.config.Server.ShutdownTimeout,
	)
	return s.httpServer.Shutdown(ctx)
}

// Addr returns the address the server is listening on
func (s *Server) Addr() string {
	return s.httpServer.Addr
}

// errorLogWriter adapts our structured logger for http.Server.ErrorLog
// This allows the stdlib HTTP server to use our structured logger
type errorLogWriter struct {
	logger *observability.Logger
}

// The stdlib http.Server will call this to log internal errors
func (w *errorLogWriter) Write(p []byte) (n int, err error) {
	// Convert bytes to string and remove trailing newline
	message := string(p)
	if len(message) > 0 && message[len(message)-1] == '\n' {
		message = message[:len(message)-1]
	}

	w.logger.Error("http server error", "message", message)
	return len(p), nil
}
