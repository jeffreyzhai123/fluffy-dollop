package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/config"
)

// Logger wraps slog.Logger with additional context-aware functionality
type Logger struct {
	*slog.Logger
	serviceName string
}

// ContextKey is the type for context keys used in logging
type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	TraceIDKey   ContextKey = "trace_id"
	UserIDKey    ContextKey = "user_id"
)

// NewLogger creates a new logger instance based on configuration
func NewLogger(cfg *config.ObservabilityConfig, serviceName string) *Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level:     parseLogLevel(cfg.LogLevel),
		AddSource: parseLogLevel(cfg.LogLevel) <= slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				if t, ok := a.Value.Any().(time.Time); ok {
					a.Value = slog.StringValue(t.Format(time.RFC3339))
				}
			}
			return a
		},
	}

	// Select handler based on log format
	var output io.Writer = os.Stdout
	switch cfg.LogFormat {
	case "json":
		handler = slog.NewJSONHandler(output, opts)
	case "text":
		handler = slog.NewTextHandler(output, opts)
	default:
		// Default to JSON for production
		handler = slog.NewJSONHandler(output, opts)
	}

	// Create base logger with service name
	baseLogger := slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("environment", getEnv("ENVIRONMENT", "development")),
	)

	return &Logger{
		Logger:      baseLogger,
		serviceName: serviceName,
	}
}

// WithContext extracts contextual information and adds it to the logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l.Logger

	// Extract request ID if present
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		logger = logger.With(slog.String("request_id", requestID))
	}

	// Extract trace ID if present
	if traceID, ok := ctx.Value(TraceIDKey).(string); ok && traceID != "" {
		logger = logger.With(slog.String("trace_id", traceID))
	}

	// Extract user ID if present
	if userID, ok := ctx.Value(UserIDKey).(string); ok && userID != "" {
		logger = logger.With(slog.String("user_id", userID))
	}

	return &Logger{
		Logger:      logger,
		serviceName: l.serviceName,
	}
}

// With returns a new logger with additional fields
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger:      l.Logger.With(args...),
		serviceName: l.serviceName,
	}
}

// WithError returns a new logger with an error field
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{
		Logger:      l.Logger.With(slog.String("error", err.Error())),
		serviceName: l.serviceName,
	}
}

// WithFields returns a new logger with multiple structured fields
func (l *Logger) WithFields(fields map[string]any) *Logger {
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	return &Logger{
		Logger:      l.Logger.With(attrs...),
		serviceName: l.serviceName,
	}
}

// LogHTTPRequest logs an incoming HTTP request
func (l *Logger) LogHTTPRequest(method, path string, statusCode int, duration time.Duration, size int64) {
	l.Info("http request",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", statusCode),
		slog.Duration("duration", duration),
		slog.Int64("size", size),
	)
}

// LogHTTPError logs an HTTP error with details
func (l *Logger) LogHTTPError(method, path string, statusCode int, err error) {
	l.Error("http error",
		slog.String("method", method),
		slog.String("path", path),
		slog.Int("status", statusCode),
		slog.String("error", err.Error()),
	)
}

// LogQueueEnqueue logs a successful queue enqueue operation
func (l *Logger) LogQueueEnqueue(streamName, messageID string, payloadSize int) {
	l.Debug("enqueued message",
		slog.String("stream", streamName),
		slog.String("message_id", messageID),
		slog.Int("payload_size", payloadSize),
	)
}

// LogQueueDequeue logs a successful queue dequeue operation
func (l *Logger) LogQueueDequeue(streamName, messageID string, jobID string, retryCount int) {
	l.Debug("dequeued message",
		slog.String("stream", streamName),
		slog.String("message_id", messageID),
		slog.String("job_id", jobID),
		slog.Int("retry_count", retryCount),
	)
}

// LogQueueError logs a queue operation error
func (l *Logger) LogQueueError(operation string, err error) {
	l.Error("queue error",
		slog.String("operation", operation),
		slog.String("error", err.Error()),
	)
}

// LogDBQuery logs a database query execution
func (l *Logger) LogDBQuery(query string, duration time.Duration, rowsAffected int64) {
	l.Debug("db query",
		slog.String("query", query),
		slog.Duration("duration", duration),
		slog.Int64("rows_affected", rowsAffected),
	)
}

// LogDBError logs a database error
func (l *Logger) LogDBError(operation string, err error) {
	l.Error("db error",
		slog.String("operation", operation),
		slog.String("error", err.Error()),
	)
}

// parseLogLevel converts string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// getEnv is a helper to get environment variable with default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
