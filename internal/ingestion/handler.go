package ingestion

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
	"github.com/jeffreyzhai123/fluffy-dollop/pkg/models"
)

type Handler struct {
	logger  *observability.Logger
	service *Service
}

func NewHandler(logger *observability.Logger, service *Service) *Handler {
	return &Handler{
		logger:  logger,
		service: service,
	}
}

// TEMP (add real logic later)
func (h *Handler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	log := h.logger.WithContext(r.Context())
	log.Info("log ingestion request received")

	var logEvent models.LogEvent
	if err := json.NewDecoder(r.Body).Decode(&logEvent); err != nil {
		respondError(w, log, http.StatusBadRequest, "invalid request body")
		return
	}

	if logEvent.Timestamp.IsZero() {
		logEvent.Timestamp = time.Now()
	}

	messageID, err := h.service.IngestLog(r.Context(), &logEvent)
	if err != nil {
		// Handle specific errors with appropriate HTTP codes
		var rateLimitErr *ErrRateLimited
		if errors.As(err, &rateLimitErr) {
			retryAfterSec := int(rateLimitErr.RetryAfter.Seconds())
			w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))
			respondError(w, log, http.StatusTooManyRequests,
				fmt.Sprintf("rate limited - retry after %d seconds", retryAfterSec))
			return
		}

		if errors.Is(err, ErrCircuitOpen) {
			respondError(w, log, http.StatusServiceUnavailable,
				"system overloaded - try again later")
			return
		}

		if errors.Is(err, ErrValidationFailed) {
			respondError(w, log, http.StatusBadRequest, "invalid log event")
			return
		}

		log.Error("ingestion failed", "error", err)
		respondError(w, log, http.StatusInternalServerError, "failed to process log")
		return
	}

	log.Info("log enqueued successfully", "message_id", messageID)
	respondJSON(w, log, http.StatusAccepted, IngestResponse{
		Accepted: true,
		ID:       messageID,
	})
}

// TEMP
func (h *Handler) HandleTestFailure(w http.ResponseWriter, r *http.Request) {
	log := h.logger.WithContext(r.Context())
	log.Warn("TEST ENDPOINT CALLED - simulating failure scenario")

	testLog := &models.LogEvent{
		Timestamp: time.Now(),
		Level:     "INFO",
		Service:   "test-service",
		Message:   "Hardcoded test log event from ingestion",
		Metadata: map[string]interface{}{
			"force_failure": true,
		},
		TraceID: "trace-test-456",
	}

	messageID, err := h.service.IngestLog(r.Context(), testLog)
	if err != nil {
		log.Error("failed to ingest log", "error", err)
		respondError(w, log, http.StatusInternalServerError, "failed to ingest log")
		return
	}

	log.Info("log enqueued successfully", "message_id", messageID)
	respondJSON(w, log, http.StatusAccepted, IngestResponse{
		Accepted: true,
		ID:       messageID,
	})
}

// func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request)

// TODO: Parse request body into LogEvent
// testLog := &models.LogEvent{
// 	Timestamp: time.Now(),
// 	Level:     "INFO",
// 	Service:   "test-service",
// 	Message:   "Hardcoded test log event from ingestion",
// 	Metadata: map[string]interface{}{
// 		"test":       true,
// 		"request_id": "hardcoded-123",
// 	},
// 	TraceID: "trace-test-456",
// }
