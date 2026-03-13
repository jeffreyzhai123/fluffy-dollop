package health

import (
	"encoding/json"
	"net/http"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// HTTPHandler provides HTTP handlers for health check endpoints
type HTTPHandler struct {
	aggregator *Aggregator
	logger     *observability.Logger
}

// NewHTTPHandler creates a new health check HTTP handler
func NewHTTPHandler(aggregator *Aggregator, logger *observability.Logger) *HTTPHandler {
	return &HTTPHandler{
		aggregator: aggregator,
		logger:     logger,
	}
}

// HandleLive handles liveness probe requests (is the service running?)
func (h *HTTPHandler) HandleLive(w http.ResponseWriter, r *http.Request) {
	result := h.aggregator.CheckLiveness(r.Context())
	respondJSON(w, http.StatusOK, result) // Always 200 for liveness
}

// HandleReady handles readiness probe requests (is the service ready to accept traffic?)
func (h *HTTPHandler) HandleReady(w http.ResponseWriter, r *http.Request) {
	result := h.aggregator.CheckReadiness(r.Context())

	status := http.StatusOK
	if !result.Overall {
		h.logger.Warn("readiness check failed", "checks", result.Checks)
		status = http.StatusServiceUnavailable
	}

	respondJSON(w, status, result)
}

// RegisterRoutes registers health check routes on the provided mux
func RegisterRoutes(mux *http.ServeMux, handler *HTTPHandler) {
	mux.HandleFunc("GET /health/live", handler.HandleLive)
	mux.HandleFunc("GET /health/ready", handler.HandleReady)
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
