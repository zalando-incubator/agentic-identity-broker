// Package http provides HTTP server adapters for the identity broker.
package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/telemetryhttp"
)

// LoggingMiddleware logPrefixes defines a whitelist of path prefixes that should be logged (e.g. "/api/").
// Requests whose path does not start with any of the given prefixes are silently passed
// through without producing a log entry, which avoids noise from infrastructure probes
// (e.g. /health) and static-asset serving.
// When no prefixes are provided every request is logged.
func LoggingMiddleware(logger *slog.Logger, logPrefixes ...string) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(logPrefixes) > 0 {
				matched := false
				for _, prefix := range logPrefixes {
					if strings.HasPrefix(r.URL.Path, prefix) {
						matched = true
						break
					}
				}
				if !matched {
					next.ServeHTTP(w, r)
					return
				}
			}

			start := time.Now()

			// Wrap ResponseWriter to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Call the next handler
			next.ServeHTTP(wrapped, r)
			httpmiddleware.FinalizeRequestSecurityContext(r.Context())

			// Log after the request completes
			duration := time.Since(start)
			clientIP := sanitizeLogValue(clientIPFromContext(r.Context(), r.RemoteAddr))
			method := r.Method
			if r.URL.Path == "/oauth2/token" {
				method = telemetryhttp.MethodName(method)
			}
			logger.InfoContext(r.Context(), "HTTP request",
				"method", sanitizeLogValue(method),
				"path", sanitizeLogValue(r.URL.Path),
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"remote_addr", clientIP,
				"client_ip", clientIP,
			)
		})
	}
}

// RecoveryMiddleware returns a middleware that recovers from panics and logs them.
// Prevents the entire server from crashing due to a single request panic.
// traceResponseEnabled gates the traceresponse header on the recovered 500 path so
// operators who set trace.response_enabled=false are honored on panics too.
func RecoveryMiddleware(logger *slog.Logger, traceResponseEnabled bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					writeTraceResponseHeaderIfPresent(w, r.Context(), traceResponseEnabled)
					if r.URL.Path == "/oauth2/token" {
						logger.ErrorContext(r.Context(), "Panic recovered", "error_kind", "internal_unclassified", "operation", "token_endpoint")
					} else {
						clientIP := sanitizeLogValue(clientIPFromContext(r.Context(), r.RemoteAddr))
						logger.ErrorContext(r.Context(), "Panic recovered",
							"error", sanitizeLogValue(fmt.Sprint(err)),
							"method", sanitizeLogValue(r.Method),
							"path", sanitizeLogValue(r.URL.Path),
							"remote_addr", clientIP,
							"client_ip", clientIP,
							"stack", string(debug.Stack()),
						)
					}

					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func ContextRecoveryMiddleware(logger *slog.Logger, traceResponseEnabled bool) func(next http.Handler) http.Handler {
	return RecoveryMiddleware(logger, traceResponseEnabled)
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before writing it.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func sanitizeLogValue(value string) string {
	if !strings.ContainsAny(value, "\\\r\n\v\f\u0085\u2028\u2029") {
		return value
	}
	quoted := strconv.Quote(value)
	return quoted[1 : len(quoted)-1]
}

func clientIPFromContext(ctx context.Context, fallback string) string {
	if sc, ok := security.FromContext(ctx); ok && sc.ClientIP != "" {
		return sc.ClientIP
	}

	return fallback
}

func writeTraceResponseHeaderIfPresent(w http.ResponseWriter, ctx context.Context, traceResponseEnabled bool) {
	httpmiddleware.FinalizeRequestSecurityContext(ctx)
	if !traceResponseEnabled {
		return
	}
	if sc, ok := security.FromContext(ctx); ok {
		w.Header().Set("traceresponse", httpmiddleware.TraceResponseValue(ctx, sc.TraceID))
	}
}
