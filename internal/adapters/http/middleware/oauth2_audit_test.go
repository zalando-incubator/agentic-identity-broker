package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOAuth2AuditMiddleware_LogsRequestID tests that middleware extracts and logs request ID
func TestOAuth2AuditMiddleware_LogsRequestID(t *testing.T) {
	// Create a buffer to capture log output
	var logBuf []slog.Record
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: &logBuf,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	// Create a test handler that will be wrapped
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrappedHandler := middleware(nextHandler)

	// Make request with request ID header
	req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize", nil)
	req.Header.Set("X-Request-ID", "req-123")
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestOAuth2AuditMiddleware_ExtractsPrincipal tests that middleware reads principal from context
func TestOAuth2AuditMiddleware_ExtractsPrincipal(t *testing.T) {
	var logBuf []slog.Record
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: &logBuf,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	var principalFromContext id.Principal
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract principal from request context using typed context key
		principalValue, _ := principal.FromContext(r.Context())
		principalFromContext = principalValue
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	// Simulate RequirePrincipalMiddleware setting principal in context
	req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize", nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, id.Principal("user@example.com"), principalFromContext)
}

// TestOAuth2AuditMiddleware_LogsRequestDetails tests that middleware logs request method, path, status
func TestOAuth2AuditMiddleware_LogsRequestDetails(t *testing.T) {
	var logBuf []slog.Record
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: &logBuf,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize?client_id=test", nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	req.Header.Set("X-Request-ID", "req-123")
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestOAuth2AuditMiddleware_LogsErrorResponses tests that middleware logs 4xx and 5xx responses
func TestOAuth2AuditMiddleware_LogsErrorResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"4xx error", http.StatusBadRequest},
		{"5xx error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logBuf []slog.Record
			handler := slog.DiscardHandler
			testHandler := &testLogHandler{
				records: &logBuf,
				wrapped: handler,
			}
			logger := slog.New(testHandler)

			middleware := OAuth2AuditMiddleware(logger)

			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			})

			wrappedHandler := middleware(nextHandler)

			req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize", nil)
			req.Header.Set("X-Remote-User", "user@example.com")
			w := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

// TestOAuth2AuditMiddleware_NoSensitiveDataLogged tests that sensitive OAuth2 parameters not logged
func TestOAuth2AuditMiddleware_NoSensitiveDataLogged(t *testing.T) {
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: nil,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	// Request with sensitive parameters
	req := httptest.NewRequest(
		"GET",
		"https://broker.example.com/oauth2/authorize?client_id=test&client_secret=secret123&authorization_code=code456",
		nil,
	)
	req.Header.Set("X-Remote-User", "user@example.com")
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Middleware should not log sensitive parameters in URL
}

// TestOAuth2AuditMiddleware_HandlesNoPrincipalHeader tests middleware when X-Remote-User header missing
func TestOAuth2AuditMiddleware_HandlesNoPrincipalHeader(t *testing.T) {
	var logBuf []slog.Record
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: &logBuf,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	// Request without X-Remote-User header
	req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize", nil)
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// TestOAuth2AuditMiddleware_PreservesRequestPath tests middleware preserves original request path
func TestOAuth2AuditMiddleware_PreservesRequestPath(t *testing.T) {
	var logBuf []slog.Record
	handler := slog.DiscardHandler
	testHandler := &testLogHandler{
		records: &logBuf,
		wrapped: handler,
	}
	logger := slog.New(testHandler)

	middleware := OAuth2AuditMiddleware(logger)

	capturedPath := ""
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware(nextHandler)

	req := httptest.NewRequest("GET", "https://broker.example.com/oauth2/authorize", nil)
	req.Header.Set("X-Remote-User", "user@example.com")
	w := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/oauth2/authorize", capturedPath)
}

func TestOAuth2AuditMiddleware_UsesBoundedSecurityContextUserAgent(t *testing.T) {
	var records []slog.Record
	testHandler := &testLogHandler{
		records: &records,
		wrapped: slog.DiscardHandler,
	}
	logger := slog.New(telemetry.NewContextHandler(testHandler))
	userAgent := strings.Repeat("a", security.MaxUserAgentByte+1)
	req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/authorize", nil)
	req.Header.Set("User-Agent", userAgent)
	req = req.WithContext(security.WithSecurityContext(req.Context(), security.SecurityContext{
		UserAgent: security.TruncateUserAgent(userAgent),
	}))

	OAuth2AuditMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(httptest.NewRecorder(), req)

	require.Len(t, records, 1)
	attrs := make(map[string]any, records[0].NumAttrs())
	records[0].Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	assert.Equal(t, security.TruncateUserAgent(userAgent), attrs["user_agent"])
}

// testLogHandler is a test slog.Handler that captures log records
type testLogHandler struct {
	records *[]slog.Record
	wrapped slog.Handler
	onLog   func(string)
}

func (h *testLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *testLogHandler) Handle(ctx context.Context, record slog.Record) error {
	if h.records != nil {
		*h.records = append(*h.records, record)
	}
	if h.onLog != nil {
		// Log the record message
		h.onLog(record.Message)
	}
	return h.wrapped.Handle(ctx, record)
}

func (h *testLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *testLogHandler) WithGroup(name string) slog.Handler {
	return h
}

func serveAudit(t *testing.T, req *http.Request) (map[string]any, string) {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	contextRequestID := ""
	OAuth2AuditMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextRequestID = httpctx.RequestIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(httptest.NewRecorder(), req)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	require.Len(t, lines, 1)
	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	assert.Equal(t, record["request_id"], contextRequestID)
	return record, contextRequestID
}

func TestOAuth2AuditMiddleware_EscapesLineBreaksInPath(t *testing.T) {
	for _, tt := range []struct {
		name string
		path string
		want string
	}{
		{"CR LF", "/oauth2/authorize\r\nforged=1", `/oauth2/authorize\r\nforged=1`},
		{"line separator", "/oauth2/authorize\u2028forged=1", `/oauth2/authorize\u2028forged=1`},
		{"paragraph separator", "/oauth2/authorize\u2029forged=1", `/oauth2/authorize\u2029forged=1`},
		{"next line", "/oauth2/authorize\u0085forged=1", `/oauth2/authorize\u0085forged=1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/authorize", nil)
			req.URL.Path = tt.path

			record, _ := serveAudit(t, req)

			assert.Equal(t, tt.want, record["path"])
		})
	}
}

func TestOAuth2AuditMiddleware_KeepsValidRequestID(t *testing.T) {
	for _, requestID := range []string{"req-123", "a", "0f8fad5b-d9cb-469f-a165-70867728950e", "trace.ID_1", strings.Repeat("a", maxRequestIDLength)} {
		t.Run(requestID, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/authorize", nil)
			req.Header.Set("X-Request-ID", requestID)

			record, contextRequestID := serveAudit(t, req)

			assert.Equal(t, requestID, record["request_id"])
			assert.Equal(t, requestID, contextRequestID)
		})
	}
}

func TestOAuth2AuditMiddleware_ReplacesInvalidRequestID(t *testing.T) {
	for _, tt := range []struct {
		name      string
		requestID string
	}{
		{"too long", strings.Repeat("a", maxRequestIDLength+1)},
		{"line break", "req-123\nforged=1"},
		{"line separator", "req-123\u2028forged"},
		{"space", "req 123"},
		{"quote", `req"123`},
		{"non-ascii", "req-ü"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/authorize", nil)
			req.Header.Set("X-Request-ID", tt.requestID)

			record, contextRequestID := serveAudit(t, req)

			assert.NotEqual(t, tt.requestID, record["request_id"])
			assertGeneratedRequestID(t, contextRequestID)
		})
	}
}

func TestOAuth2AuditMiddleware_GeneratesReadableUniqueRequestIDs(t *testing.T) {
	seen := make(map[string]struct{})
	for range 1000 {
		req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/authorize", nil)

		_, requestID := serveAudit(t, req)

		assertGeneratedRequestID(t, requestID)
		_, duplicate := seen[requestID]
		require.False(t, duplicate, "duplicate request ID %q", requestID)
		seen[requestID] = struct{}{}
	}
}

func assertGeneratedRequestID(t *testing.T, requestID string) {
	t.Helper()
	parsed, err := uuid.Parse(requestID)
	require.NoError(t, err, "request ID %q", requestID)
	assert.Equal(t, parsed.String(), requestID)
}

func TestOAuth2AuditMiddleware_LogsSecurityContextRequestLine(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://broker.example.com/oauth2/a%2Fb", nil)
	capture := security.TransportCapture{RequestMethod: http.MethodGet, RequestTarget: "/oauth2/a%2Fb"}
	req = req.WithContext(security.WithCaptureHolder(req.Context(), security.NewCaptureHolder(capture)))

	record, _ := serveAudit(t, req)

	assert.Equal(t, http.MethodGet, record["method"])
	assert.Equal(t, "/oauth2/a%2Fb", record["path"])
}
