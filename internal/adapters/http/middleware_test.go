// Package http provides HTTP server adapters for the identity broker.
package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func TestLoggingMiddlewareLogsWhitelistedPrefix(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := LoggingMiddleware(logger, "/api/")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/something", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !strings.Contains(buf.String(), "HTTP request") {
		t.Errorf("expected log output for /api/ path, got: %s", buf.String())
	}
}

func TestLoggingMiddlewareSkipsNonWhitelistedPaths(t *testing.T) {
	paths := []string{"/health", "/consent/index.html", "/favicon.ico"}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&buf, nil))

			handler := LoggingMiddleware(logger, "/api/", "/oauth2/", "/.well-known/")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
			}
			if buf.Len() != 0 {
				t.Errorf("expected no log output for %q, got: %s", path, buf.String())
			}
		})
	}
}

func TestLoggingMiddlewareLogsAllPathsWhenNoPrefixesGiven(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	handler := LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !strings.Contains(buf.String(), "HTTP request") {
		t.Errorf("expected log output when no prefixes given, got: %s", buf.String())
	}
}

func TestMiddlewareSanitizesLogValues(t *testing.T) {
	for _, tc := range []struct {
		name       string
		middleware func(*slog.Logger) func(http.Handler) http.Handler
		panicValue func(string) any
		status     int
	}{
		{"access", func(logger *slog.Logger) func(http.Handler) http.Handler { return LoggingMiddleware(logger) }, nil, http.StatusNoContent},
		{"panic string", func(logger *slog.Logger) func(http.Handler) http.Handler { return RecoveryMiddleware(logger, false) }, func(s string) any { return s }, http.StatusInternalServerError},
		{"panic error", func(logger *slog.Logger) func(http.Handler) http.Handler { return RecoveryMiddleware(logger, false) }, func(s string) any { return errors.New(s) }, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, lineBreak := range []string{"", "\r", "\n", "\r\n", "\v", "\f", "\u0085", "\u2028", "\u2029"} {
				t.Run(fmt.Sprintf("line break %q", lineBreak), func(t *testing.T) {
					for _, source := range []string{"remote address", "context client IP"} {
						t.Run(source, func(t *testing.T) {
							base := newPanicLogCaptureHandler()
							logger := slog.New(base)
							method := "GE" + lineBreak + "T"
							path := "/api/" + lineBreak + "resource"
							clientIP := "192.0.2." + lineBreak + "1"
							panicMessage := "invalid " + lineBreak + "input"
							handler := tc.middleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
								assert.Equal(t, method, r.Method)
								assert.Equal(t, path, r.URL.Path)
								if tc.panicValue != nil {
									panic(tc.panicValue(panicMessage))
								}
								w.WriteHeader(tc.status)
							}))
							req := httptest.NewRequest(http.MethodGet, "/api/resource", nil)
							req.Method = method
							req.URL.Path = path
							req.RemoteAddr = clientIP
							if source == "context client IP" {
								req.RemoteAddr = "198.51.100.2:1234"
								req = req.WithContext(security.WithSecurityContext(req.Context(), security.SecurityContext{ClientIP: clientIP}))
							}
							rr := httptest.NewRecorder()
							handler.ServeHTTP(rr, req)

							assert.Equal(t, tc.status, rr.Code)
							require.Len(t, *base.records, 1)
							attrs := (*base.records)[0].attrs
							assert.Equal(t, "GET", attrs["method"])
							assert.Equal(t, "/api/resource", attrs["path"])
							assert.Equal(t, "192.0.2.1", attrs["remote_addr"])
							assert.Equal(t, "192.0.2.1", attrs["client_ip"])
							if tc.panicValue != nil {
								assert.Equal(t, "invalid input", attrs["error"])
							}
						})
					}
				})
			}
		})
	}
}

func TestFirstValidTraceparent_IgnoresOtherPropagationFormats(t *testing.T) {
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, b3.New()))
	t.Cleanup(func() { otel.SetTextMapPropagator(previousPropagator) })

	const validTraceparent = "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01"
	header := http.Header{}
	header.Add("traceparent", "not-a-valid-traceparent")
	header.Add("traceparent", validTraceparent)
	header.Set("b3", "cccccccccccccccccccccccccccccccc-dddddddddddddddd-1")

	got, ok := firstValidTraceparent(header, header.Values("traceparent"))
	require.True(t, ok)
	assert.Equal(t, validTraceparent, got)
}

type panicLogRecord struct {
	message string
	attrs   map[string]any
}

type panicLogCaptureHandler struct {
	records *[]panicLogRecord
	attrs   []slog.Attr
	groups  []string
}

func newPanicLogCaptureHandler() *panicLogCaptureHandler {
	records := make([]panicLogRecord, 0, 1)
	return &panicLogCaptureHandler{records: &records}
}

func (h *panicLogCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *panicLogCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any, record.NumAttrs()+len(h.attrs))
	for _, attr := range h.attrs {
		h.storeAttr(attrs, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		h.storeAttr(attrs, attr)
		return true
	})
	*h.records = append(*h.records, panicLogRecord{message: record.Message, attrs: attrs})
	return nil
}

func (h *panicLogCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *panicLogCaptureHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *panicLogCaptureHandler) storeAttr(dst map[string]any, attr slog.Attr) {
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string{}, h.groups...), attr.Key), ".")
	}
	dst[key] = attr.Value.Any()
}

func TestRecoveryMiddleware_PanicLogCarriesSecurityContextAndTraceHeader(t *testing.T) {
	t.Parallel()

	base := newPanicLogCaptureHandler()
	logger := slog.New(telemetry.NewContextHandler(base))
	handler := RecoveryMiddleware(logger, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/panic", nil).WithContext(security.WithSecurityContext(
		context.Background(),
		security.SecurityContext{
			TraceID:     "0123456789abcdef0123456789abcdef",
			Actor:       "alice@example.com",
			CallingPeer: "gateway-client-1",
		},
	))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Len(t, *base.records, 1)
	assert.Equal(t, "Panic recovered", (*base.records)[0].message)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", (*base.records)[0].attrs["trace_id"], "panic log must retain request trace correlation")
	assert.Equal(t, "alice@example.com", (*base.records)[0].attrs["actor"], "panic log must retain the finalized actor")
	assert.Equal(t, "gateway-client-1", (*base.records)[0].attrs["calling_peer"], "panic log must retain the finalized calling_peer")
	assert.NotEmpty(t, rr.Header().Get("traceresponse"), "recovered 500 responses must still carry traceresponse")
}

func TestRecoveryMiddleware_SuppressesTraceResponseHeaderWhenDisabled(t *testing.T) {
	t.Parallel()

	base := newPanicLogCaptureHandler()
	logger := slog.New(telemetry.NewContextHandler(base))
	handler := RecoveryMiddleware(logger, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/panic", nil).WithContext(security.WithSecurityContext(
		context.Background(),
		security.SecurityContext{
			TraceID: "0123456789abcdef0123456789abcdef",
			Actor:   "alice@example.com",
		},
	))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	require.Len(t, *base.records, 1)
	assert.Equal(t, "0123456789abcdef0123456789abcdef", (*base.records)[0].attrs["trace_id"],
		"panic log must still retain trace correlation even when the response header is disabled")
	assert.Empty(t, rr.Header().Get("traceresponse"),
		"recovered 500 responses must honor trace.response_enabled=false and suppress the traceresponse header")
}
