package middleware

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/google/uuid"
)

// OAuth2AuditMiddleware creates audit logging middleware for OAuth2 endpoints
func OAuth2AuditMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if !validRequestID(requestID) {
				requestID = uuid.NewString()
			}

			// Extract principal from context (set by RequirePrincipalMiddleware)
			// Note: May be empty for public endpoints (metadata, token proxy)
			principalValue, _ := principal.FromContext(r.Context())

			// Record request start time
			startTime := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriterWrapper{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Add request_id to context
			ctx := httpctx.WithRequestID(r.Context(), requestID)

			// Call next handler with updated context
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Log audit information
			duration := time.Since(startTime).Milliseconds()
			method, target := LoggedRequestLine(r.WithContext(ctx))
			logAuditEvent(ctx, logger, &auditEvent{
				RequestID:  requestID,
				Principal:  principalValue,
				Method:     SanitizeLogValue(method),
				Path:       SanitizeLogValue(target),
				StatusCode: wrapped.statusCode,
				Duration:   duration,
				RemoteAddr: auditRemoteAddr(ctx, r.RemoteAddr),
				UserAgent:  auditUserAgent(ctx, r.UserAgent()),
			})
		})
	}
}

// responseWriterWrapper wraps http.ResponseWriter to capture status code
type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (w *responseWriterWrapper) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriterWrapper) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

// auditEvent contains audit information about a request
type auditEvent struct {
	RequestID  string
	Principal  string
	Method     string
	Path       string
	StatusCode int
	Duration   int64
	RemoteAddr string
	UserAgent  string
}

// logAuditEvent logs an audit event with structured logging
func logAuditEvent(ctx context.Context, logger *slog.Logger, event *auditEvent) {
	// Determine log level based on status code
	level := slog.LevelInfo
	if event.StatusCode >= 400 && event.StatusCode < 500 {
		level = slog.LevelWarn
	} else if event.StatusCode >= 500 {
		level = slog.LevelError
	}

	// Get the remote host without port
	host, _, _ := net.SplitHostPort(event.RemoteAddr)
	if host == "" {
		host = event.RemoteAddr
	}

	logger.Log(
		ctx,
		level,
		"OAuth2 Authorization Request",
		slog.String("request_id", event.RequestID),
		slog.String("principal", event.Principal),
		slog.String("method", event.Method),
		slog.String("path", event.Path),
		slog.Int("status", event.StatusCode),
		slog.Int64("duration_ms", event.Duration),
		slog.String("remote_host", host),
		slog.String("user_agent", event.UserAgent),
	)
}

func auditRemoteAddr(ctx context.Context, fallback string) string {
	if sc, ok := security.FromContext(ctx); ok && sc.ClientIP != "" {
		return sc.ClientIP
	}

	return fallback
}

func auditUserAgent(ctx context.Context, fallback string) string {
	if sc, ok := security.FromContext(ctx); ok {
		return sc.UserAgent
	}

	return security.TruncateUserAgent(fallback)
}

const maxRequestIDLength = 128

// validRequestID accepts a caller-supplied X-Request-ID only when it is 1 to
// maxRequestIDLength characters of [A-Za-z0-9._-]; anything else is replaced so it is never logged.
func validRequestID(value string) bool {
	if value == "" || len(value) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '.' && c != '_' && c != '-' {
			return false
		}
	}
	return true
}
