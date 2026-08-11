package middleware

import (
	"net/http"
	"time"

	"github.com/vamshireddy02/mindova/packages/kernel/logger"
)

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (w *statusResponseWriter) WriteHeader(statusCode int) {
	if !w.written {
		w.statusCode = statusCode
		w.written = true
		w.ResponseWriter.WriteHeader(statusCode)
	}
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.statusCode = http.StatusOK
		w.written = true
	}
	return w.ResponseWriter.Write(b)
}

func RequestLogging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Start timer
			start := time.Now()

			// Get request ID from context (set by RequestID middleware)
			requestID := GetRequestID(r.Context())

			// Wrap ResponseWriter to capture status code
			wrapped := &statusResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default if not explicitly set
				written:        false,
			}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			log.Info("request completed",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
			)
		})
	}
}
