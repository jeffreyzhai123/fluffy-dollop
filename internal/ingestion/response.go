package ingestion

import (
	"encoding/json"
	"net/http"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// IngestResponse is returned when a log is successfully accepted
type IngestResponse struct {
	Accepted bool   `json:"accepted"`
	ID       string `json:"id,omitempty"`
}

// ErrorResponse is returned for all error cases
type ErrorResponse struct {
	Error string `json:"error"`
}

// respondJSON writes a JSON response with the given status code
func respondJSON(w http.ResponseWriter, logger *observability.Logger, status int, data any) {
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		logger.Error("failed to encode JSON response",
			"error", err,
			"status", status,
		)
	}
}

// respondError writes a JSON error response
func respondError(w http.ResponseWriter, logger *observability.Logger, status int, message string) {
	errorResp := ErrorResponse{
		Error: message,
	}

	logger.Warn("returning error response",
		"status", status,
		"error", message,
	)

	respondJSON(w, logger, status, errorResp)
}
