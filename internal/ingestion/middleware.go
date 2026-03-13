package ingestion

import (
	"net/http"
	"time"

	"github.com/jeffreyzhai123/fluffy-dollop/internal/observability"
)

// loggingMiddleware logs all HTTP requests with timing and status
func loggingMiddleware(logger *observability.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Log the request
		logger.LogHTTPRequest(
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start),
			wrapped.bytesWritten,
		)
	})
}

// requestIDMiddleware adds a unique request ID to the context
// func requestIDMiddleware(next http.Handler) http.Handler {
// }

// recoverMiddleware recovers from panics and returns 500
// func recoverMiddleware(logger *observability.Logger, next http.Handler) http.Handler {
// }

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}
