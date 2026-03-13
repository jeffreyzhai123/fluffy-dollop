package models

import "time"

// LogEvent represents a single log entry from an external service
// This is the domain model - what the API accepts
type LogEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level" validate:"required,oneof=DEBUG INFO WARN ERROR"`
	Service   string                 `json:"service" validate:"required"`
	Message   string                 `json:"message" validate:"required"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}
