// Package middleware provides HTTP middleware for the distributed caching system.
//
// This file implements structured request logging middleware with:
//   - Request/response logging with timing
//   - Correlation ID propagation (X-Request-ID header)
//   - Context-based request ID storage
//   - JSON structured logging
//   - Low-overhead design for hot paths
//
// Design Notes:
//   - Uses standard log package for compatibility
//   - Correlation IDs enable distributed tracing across services
//   - Request IDs stored in context for downstream use
//   - Logs include method, path, status, duration, size
//
// Trade-offs:
//   - Structured JSON logging vs human-readable: chose JSON for parsing
//   - fmt.Sprintf avoided in hot path: use strings.Builder where needed
//   - Log level: Info for success, Warn for 4xx, Error for 5xx
//
// Production extensions:
//   - Integrate with zerolog/zap for higher performance
//   - Add sampling for high-traffic endpoints
//   - Send logs to centralized logging (e.g., DataDog, ELK)
//   - Add request body logging (with PII redaction)
package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ContextKey type for context keys to avoid collisions
type contextKey string

const (
	// RequestIDKey is the context key for request IDs
	requestIDKey contextKey = "request-id"
)

// RequestLogger is a middleware that logs HTTP requests with structured logging.
//
// Example usage:
//
//	mux := http.NewServeMux()
//	loggedMux := RequestLogger(mux)
//	http.ListenAndServe(":8080", loggedMux)
//
// Logs include:
//   - Request ID (from X-Request-ID header or generated)
//   - HTTP method and path
//   - Response status code
//   - Response size in bytes
//   - Duration in milliseconds
//   - Remote address
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract or generate request ID
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Store request ID in context
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		r = r.WithContext(ctx)

		// Set request ID in response header
		w.Header().Set("X-Request-ID", requestID)

		// Wrap response writer to capture status code and size
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default
		}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Calculate duration
		duration := time.Since(start)

		// Log request
		logRequest(requestID, r, wrapped.statusCode, wrapped.bytesWritten, duration)
	})
}

// WithRequestID adds a request ID to the context.
// Useful for manually propagating request IDs.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

// RequestIDFromCtx retrieves the request ID from the context.
// Returns empty string if not found.
func RequestIDFromCtx(ctx context.Context) string {
	if requestID, ok := ctx.Value(requestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// generateRequestID creates a new UUID-based request ID.
// Format: uuid v4 (e.g., "550e8400-e29b-41d4-a716-446655440000")
//
// Alternative implementations:
//   - Timestamp + counter: "20240115-123456-0001"
//   - Base64(timestamp + random): "MTYxMDQ4NzY0MA=="
func generateRequestID() string {
	return uuid.New().String()
}

// logRequest writes a structured JSON log entry.
//
// Log fields:
//   - timestamp: ISO 8601 timestamp
//   - request_id: Correlation ID
//   - method: HTTP method
//   - path: Request path
//   - status: HTTP status code
//   - duration_ms: Request duration in milliseconds
//   - bytes: Response size in bytes
//   - remote_addr: Client IP address
//   - user_agent: Client user agent
func logRequest(requestID string, r *http.Request, statusCode int, bytesWritten int, duration time.Duration) {
	logEntry := map[string]interface{}{
		"timestamp":   time.Now().UTC().Format(time.RFC3339),
		"request_id":  requestID,
		"method":      r.Method,
		"path":        r.URL.Path,
		"query":       r.URL.RawQuery,
		"status":      statusCode,
		"duration_ms": duration.Milliseconds(),
		"bytes":       bytesWritten,
		"remote_addr": r.RemoteAddr,
		"user_agent":  r.UserAgent(),
	}

	// Serialize to JSON
	data, err := json.Marshal(logEntry)
	if err != nil {
		// Fallback to simple logging if JSON marshal fails
		log.Printf("[ERROR] Failed to marshal log entry: %v", err)
		log.Printf("[%s] %s %s - %d (%dms)", requestID, r.Method, r.URL.Path, statusCode, duration.Milliseconds())
		return
	}

	// Determine log level based on status code
	if statusCode >= 500 {
		log.Printf("[ERROR] %s", string(data))
	} else if statusCode >= 400 {
		log.Printf("[WARN] %s", string(data))
	} else {
		log.Printf("[INFO] %s", string(data))
	}
}

// responseWriter wraps http.ResponseWriter to capture status code and bytes written.
type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

// WriteHeader captures the status code.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the number of bytes written.
func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// Flush implements http.Flusher interface.
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// LogWithRequestID logs a message with the request ID from context.
// Useful for application-level logging that should include correlation IDs.
//
// Example:
//
//	LogWithRequestID(ctx, "Cache hit", map[string]interface{}{"key": "user:123"})
func LogWithRequestID(ctx context.Context, message string, fields map[string]interface{}) {
	requestID := RequestIDFromCtx(ctx)

	logEntry := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"request_id": requestID,
		"message":    message,
	}

	// Merge additional fields
	for k, v := range fields {
		logEntry[k] = v
	}

	data, err := json.Marshal(logEntry)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal log entry: %v", err)
		return
	}

	log.Printf("[INFO] %s", string(data))
}