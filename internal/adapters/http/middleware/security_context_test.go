package middleware_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	adapterhttp "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var traceResponsePattern = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$`)

type observedSecurityContext struct {
	sc security.SecurityContext
	ok bool
}

type middlewareLogRecord struct {
	level   slog.Level
	message string
	attrs   map[string]any
}

type middlewareLogCaptureHandler struct {
	minLevel slog.Level
	records  *[]middlewareLogRecord
	attrs    []slog.Attr
	groups   []string
}

func newMiddlewareLogCaptureHandler(minLevel slog.Level) *middlewareLogCaptureHandler {
	records := make([]middlewareLogRecord, 0, 4)
	return &middlewareLogCaptureHandler{minLevel: minLevel, records: &records}
}

func (h *middlewareLogCaptureHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *middlewareLogCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := make(map[string]any, record.NumAttrs()+len(h.attrs))
	for _, attr := range h.attrs {
		h.storeAttr(attrs, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		h.storeAttr(attrs, attr)
		return true
	})
	*h.records = append(*h.records, middlewareLogRecord{
		level:   record.Level,
		message: record.Message,
		attrs:   attrs,
	})
	return nil
}

func (h *middlewareLogCaptureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &clone
}

func (h *middlewareLogCaptureHandler) WithGroup(name string) slog.Handler {
	clone := *h
	clone.groups = append(append([]string{}, h.groups...), name)
	return &clone
}

func (h *middlewareLogCaptureHandler) storeAttr(dst map[string]any, attr slog.Attr) {
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string{}, h.groups...), attr.Key), ".")
	}
	dst[key] = attr.Value.Any()
}

func findMiddlewareLogRecord(records []middlewareLogRecord, message string) (middlewareLogRecord, bool) {
	for _, record := range records {
		if record.message == message {
			return record, true
		}
	}
	return middlewareLogRecord{}, false
}

func newTestServerConfig() adapterhttp.ServerConfig {
	return adapterhttp.ServerConfig{
		Authentication: ports.AuthenticationConfig{
			Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
		},
	}
}

func setNestedField(t *testing.T, target any, value any, path ...string) {
	t.Helper()

	current := reflect.ValueOf(target)
	if current.Kind() != reflect.Pointer || current.Elem().Kind() != reflect.Struct {
		t.Fatalf("setNestedField target must be pointer to struct, got %T", target)
	}
	current = current.Elem()

	for i, name := range path {
		field := current.FieldByName(name)
		if !field.IsValid() {
			return
		}
		if i == len(path)-1 {
			incoming := reflect.ValueOf(value)
			if incoming.Type().AssignableTo(field.Type()) {
				field.Set(incoming)
				return
			}
			if incoming.Type().ConvertibleTo(field.Type()) {
				field.Set(incoming.Convert(field.Type()))
				return
			}
			return
		}
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				field.Set(reflect.New(field.Type().Elem()))
			}
			field = field.Elem()
		}
		if field.Kind() != reflect.Struct {
			return
		}
		current = field
	}
}

func traceIDFromTraceResponse(t *testing.T, headerValue string) string {
	t.Helper()

	matches := traceResponsePattern.FindStringSubmatch(headerValue)
	require.Len(t, matches, 2, "traceresponse header must follow W3C format")
	return matches[1]
}

func mustMiddlewareSpanContext(t *testing.T, traceIDHex, spanIDHex string) trace.SpanContext {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	require.NoError(t, err)
	spanID, err := trace.SpanIDFromHex(spanIDHex)
	require.NoError(t, err)

	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
}

func TestNewHandler_OrdinaryRequestFinalizesSecurityContextImmediately(t *testing.T) {
	t.Parallel()

	var observed observedSecurityContext
	handler := adapterhttp.NewHandler(newTestServerConfig(), func(r chi.Router) {
		r.Post("/api/token", func(w http.ResponseWriter, r *http.Request) {
			observed.sc, observed.ok = security.FromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		})
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/api/token?access_token=secret&code=opaque", nil)
	req.Header.Set("X-Remote-User", "alice@example.com")
	req.Header.Set("User-Agent", "broker-tests/1.0")
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)
	require.True(t, observed.ok, "ordinary requests must expose a finalized SecurityContext before business logic runs")
	assert.Equal(t, "alice@example.com", observed.sc.Actor)
	assert.Empty(t, observed.sc.CallingPeer, "calling_peer must be omitted when no distinct peer is present")
	assert.Equal(t, http.MethodPost, observed.sc.RequestMethod)
	assert.Equal(t, "/api/token", observed.sc.RequestTarget, "request target must omit the query string")
	assert.Equal(t, "broker-tests/1.0", observed.sc.UserAgent)
	assert.Equal(t, traceIDFromTraceResponse(t, res.Header().Get("traceresponse")), observed.sc.TraceID)
}

func TestNewHandler_AnonymousFallbackRemainsFailOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		principalHead string
	}{
		{name: "missing principal falls back to anonymous"},
		{name: "oversized principal falls back to anonymous", principalHead: strings.Repeat("p", 201)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var observed observedSecurityContext
			handler := adapterhttp.NewHandler(newTestServerConfig(), func(r chi.Router) {
				r.Get("/api/public", func(w http.ResponseWriter, r *http.Request) {
					observed.sc, observed.ok = security.FromContext(r.Context())
					w.WriteHeader(http.StatusNoContent)
				})
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))

			req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
			if tt.principalHead != "" {
				req.Header.Set("X-Remote-User", tt.principalHead)
			}
			res := httptest.NewRecorder()

			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusNoContent, res.Code, "capture must never reject a public request")
			require.True(t, observed.ok, "public requests must still receive a finalized anonymous security context")
			assert.Equal(t, security.AnonymousActor, observed.sc.Actor)
			assert.Empty(t, observed.sc.CallingPeer)
			assert.Equal(t, traceIDFromTraceResponse(t, res.Header().Get("traceresponse")), observed.sc.TraceID)
		})
	}
}

func TestNewHandler_TruncatesOversizedUserAgent(t *testing.T) {
	t.Parallel()

	var observed observedSecurityContext
	handler := adapterhttp.NewHandler(newTestServerConfig(), func(r chi.Router) {
		r.Get("/api/ua", func(w http.ResponseWriter, r *http.Request) {
			observed.sc, observed.ok = security.FromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		})
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	longUserAgent := strings.Repeat("u", 1100)
	req := httptest.NewRequest(http.MethodGet, "/api/ua", nil)
	req.Header.Set("User-Agent", longUserAgent)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)
	require.True(t, observed.ok, "requests with oversized metadata must still receive a security context")
	require.Len(t, observed.sc.UserAgent, 1024, "user-agent must be truncated to the contract limit")
	assert.Equal(t, longUserAgent[:1024], observed.sc.UserAgent)
}

func TestNewHandler_TraceIDReusesInboundSpanOrFallsBackWhenAbsent(t *testing.T) {
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(previousProvider) })

	tests := []struct {
		name           string
		requestCtx     context.Context
		tracingEnabled bool
		wantTraceID    string
	}{
		{
			name: "reuses inbound span trace id",
			requestCtx: trace.ContextWithSpanContext(context.Background(), mustMiddlewareSpanContext(
				t,
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				"bbbbbbbbbbbbbbbb",
			)),
			tracingEnabled: true,
			wantTraceID:    "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name:       "falls back when no span is present",
			requestCtx: context.Background(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var observed observedSecurityContext
			cfg := newTestServerConfig()
			cfg.Telemetry.Enabled = tt.tracingEnabled
			cfg.Telemetry.Traces.Enabled = tt.tracingEnabled
			handler := adapterhttp.NewHandler(cfg, func(r chi.Router) {
				r.Get("/api/trace", func(w http.ResponseWriter, r *http.Request) {
					observed.sc, observed.ok = security.FromContext(r.Context())
					w.WriteHeader(http.StatusNoContent)
				})
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))

			req := httptest.NewRequest(http.MethodGet, "/api/trace", nil).WithContext(tt.requestCtx)
			res := httptest.NewRecorder()
			handler.ServeHTTP(res, req)

			require.Equal(t, http.StatusNoContent, res.Code)
			require.True(t, observed.ok, "every request must have a security context trace id")
			traceID := traceIDFromTraceResponse(t, res.Header().Get("traceresponse"))
			assert.Equal(t, traceID, observed.sc.TraceID, "SecurityContext.TraceID must be the traceresponse authority")
			if tt.wantTraceID != "" {
				assert.Equal(t, tt.wantTraceID, observed.sc.TraceID)
			} else {
				assert.Regexp(t, `^[0-9a-f]{32}$`, observed.sc.TraceID, "fallback trace ids must still be W3C trace ids")
			}
		})
	}
}

func TestNewHandler_DoesNotReuseInboundTraceWhenTracingDisabled(t *testing.T) {
	const inboundTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	previousPropagator := otel.GetTextMapPropagator()
	previousProvider := otel.GetTracerProvider()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	cfg := newTestServerConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Traces.Enabled = false
	var observed observedSecurityContext
	var propagatedTraceID string
	handler := adapterhttp.NewHandler(cfg, func(r chi.Router) {
		r.Get("/api/trace", func(w http.ResponseWriter, r *http.Request) {
			observed.sc, observed.ok = security.FromContext(r.Context())
			propagatedTraceID = trace.SpanContextFromContext(r.Context()).TraceID().String()
			w.WriteHeader(http.StatusNoContent)
		})
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/api/trace", nil)
	req.Header.Set("traceparent", "00-"+inboundTraceID+"-bbbbbbbbbbbbbbbb-01")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)
	require.True(t, observed.ok)
	assert.NotEqual(t, inboundTraceID, observed.sc.TraceID)
	assert.Regexp(t, `^[0-9a-f]{32}$`, observed.sc.TraceID)
	assert.Equal(t, inboundTraceID, propagatedTraceID, "otelchi must remain installed for telemetry instrumentation")
	traceResponseParts := strings.Split(res.Header().Get("traceresponse"), "-")
	require.Len(t, traceResponseParts, 4)
	assert.NotEqual(t, "bbbbbbbbbbbbbbbb", traceResponseParts[2])
	assert.Equal(t, "00", traceResponseParts[3])
}

func TestNewHandler_TraceResponseSuppressionKeepsTraceInContextAndLogs(t *testing.T) {
	t.Parallel()

	cfg := newTestServerConfig()
	setNestedField(t, &cfg, false, "RequestContext", "Trace", "ResponseEnabled")

	base := newMiddlewareLogCaptureHandler(slog.LevelInfo)
	logger := slog.New(telemetry.NewContextHandler(base))

	var observed observedSecurityContext
	handler := adapterhttp.NewHandler(cfg, func(r chi.Router) {
		r.Get("/api/suppressed", func(w http.ResponseWriter, r *http.Request) {
			observed.sc, observed.ok = security.FromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		})
	}, logger)

	req := httptest.NewRequest(http.MethodGet, "/api/suppressed", nil).WithContext(
		trace.ContextWithSpanContext(context.Background(), mustMiddlewareSpanContext(
			t,
			"cccccccccccccccccccccccccccccccc",
			"dddddddddddddddd",
		)),
	)
	res := httptest.NewRecorder()

	handler.ServeHTTP(res, req)

	require.Equal(t, http.StatusNoContent, res.Code)
	require.True(t, observed.ok, "disabling the response header must not disable capture")
	assert.Empty(t, res.Header().Get("traceresponse"), "trace.response_enabled=false must suppress only the header")

	accessLog, ok := findMiddlewareLogRecord(*base.records, "HTTP request")
	require.True(t, ok, "request completion must still be logged")
	assert.Equal(t, observed.sc.TraceID, accessLog.attrs["trace_id"], "trace.response_enabled=false must not disable log enrichment")
}

type benchmarkResponseWriter struct {
	header http.Header
	status int
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{header: make(http.Header)}
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(body), nil
}

func (w *benchmarkResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *benchmarkResponseWriter) Reset() {
	clear(w.header)
	w.status = 0
}

func TestNewHandler_TracingOmitsQueryBearingRequests(t *testing.T) {
	const secret = "authorization-code-sentinel"
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		assert.NoError(t, provider.Shutdown(context.Background()))
	})
	config := newTestServerConfig()
	config.Telemetry.Enabled = true
	config.Telemetry.Traces.Enabled = true
	handler := adapterhttp.NewHandler(config, func(router chi.Router) {
		router.Get("/api/third-party/{serviceId}/oauth2/callback", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	request := httptest.NewRequest(http.MethodGet, "/api/third-party/123/oauth2/callback", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
	baseline := len(recorder.Ended())
	require.Positive(t, baseline)
	request = httptest.NewRequest(http.MethodGet, "/api/third-party/123/oauth2/callback?code="+secret, nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusNoContent, response.Code)
	for _, span := range recorder.Ended()[baseline:] {
		assert.NotContains(t, fmt.Sprint(span.Attributes()), secret, "server spans must not export authorization codes")
	}
}

func BenchmarkSecurityContextMiddleware(b *testing.B) {
	cfg := ports.DefaultRequestContextConfig()
	handler := httpmiddleware.SecurityContextMiddleware(cfg, true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := security.FromContext(r.Context()); !ok {
			b.Fatal("security context missing from benchmark request")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/bench", nil)
	req = req.WithContext(principal.WithPrincipal(req.Context(), "bench@example.com"))
	req.Header.Set("User-Agent", "benchmark-client/1.0")
	req.RemoteAddr = "203.0.113.44:4321"

	writer := newBenchmarkResponseWriter()
	writer.Reset()
	handler.ServeHTTP(writer, req)
	if writer.status != http.StatusNoContent {
		b.Fatalf("warmup status = %d, want %d", writer.status, http.StatusNoContent)
	}
	if writer.Header().Get("traceresponse") == "" {
		b.Fatal("warmup traceresponse header was empty")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writer.Reset()
		handler.ServeHTTP(writer, req)
	}
}
