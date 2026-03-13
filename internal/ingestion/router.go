package ingestion

import (
	"net/http"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/health"
	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// NewRouter creates and configures the HTTP router with all routes
func NewRouter(handler *Handler, healthHandler *health.HTTPHandler, logger *observability.Logger) http.Handler {
	mux := http.NewServeMux()

	// Health check routes
	health.RegisterRoutes(mux, healthHandler)

	// API routes
	mux.HandleFunc("POST /api/v1/logs", handler.HandleLogs)
	mux.HandleFunc("POST /api/v1/test/failure", handler.HandleTestFailure)

	// Wrap with middleware (progressive wrapping)
	var h http.Handler = mux
	h = loggingMiddleware(logger, h)
	// h = requestIDMiddleware(h)
	// h = recoverMiddleware(logger, h)

	return h
}
