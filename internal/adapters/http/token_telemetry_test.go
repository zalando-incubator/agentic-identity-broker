package http

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTokenEndpointAutomaticTelemetryDoesNotSerializeCredentials(t *testing.T) {
	const secret = "SENTINEL_AUTOMATIC_HTTP_SECRET"
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider, previousPropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTracerProvider(previousProvider); otel.SetTextMapPropagator(previousPropagator) })
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := NewHandler(ServerConfig{Name: "enduser", Telemetry: ports.TelemetryConfig{Enabled: true}}, func(router chi.Router) {
		router.Post("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			assert.Equal(t, "credential="+secret, string(body))
			assert.Equal(t, secret, r.Header.Get("Authorization"))
			assert.Equal(t, secret, r.UserAgent())
			assert.Equal(t, secret+".example", r.Host)
			assert.Equal(t, secret, r.URL.Query().Get("credential"))
			w.WriteHeader(http.StatusBadRequest)
		})
	}, logger)
	server := httptest.NewServer(handler)
	defer server.Close()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/oauth2/token?credential="+secret, strings.NewReader("credential="+secret))
	require.NoError(t, err)
	request.Host = secret + ".example"
	request.Header.Set("Authorization", secret)
	request.Header.Set("User-Agent", secret)
	request.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	response, err := server.Client().Do(request)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.NotContains(t, logs.String(), secret)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	span := spans[0]
	assert.Equal(t, "POST /oauth2/token", span.Name())
	assert.Equal(t, "0123456789abcdef0123456789abcdef", span.SpanContext().TraceID().String())
	assert.Equal(t, "0123456789abcdef", span.Parent().SpanID().String())
	assert.NotContains(t, fmt.Sprint(span.Attributes(), span.Events(), span.Status(), span.Resource().Attributes()), secret)
}

func TestTokenEndpointPanicDoesNotSerializeNestedCredentials(t *testing.T) {
	const secret = "SENTINEL_TOKEN_PANIC_SECRET"
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := NewHandler(ServerConfig{Name: "enduser"}, func(router chi.Router) {
		router.Post("/oauth2/token", func(http.ResponseWriter, *http.Request) {
			panic(fmt.Errorf("provider URL https://provider.example/%s: %w", secret, fmt.Errorf("nested credential %s", secret)))
		})
	}, logger)
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Post(server.URL+"/oauth2/token?credential="+secret, "application/x-www-form-urlencoded", strings.NewReader("credential="+secret))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.NotContains(t, string(body), secret)
	assert.NotContains(t, logs.String(), secret)
	assert.Contains(t, logs.String(), `"error_kind":"internal_unclassified"`)
}

func TestTokenEndpointUnsupportedMethodTelemetryIsBounded(t *testing.T) {
	for _, tc := range []struct{ name, method, observed string }{
		{"standard method", http.MethodGet, http.MethodGet},
		{"extension method", "METHOD_SECRET_SENTINEL", "_OTHER"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() { otel.SetTracerProvider(previous) })
			var logs bytes.Buffer
			handler := NewHandler(ServerConfig{Name: "enduser", Telemetry: ports.TelemetryConfig{Enabled: true}}, func(router chi.Router) {
				router.Post("/oauth2/token", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
				router.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Observed-Method", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				})
			}, slog.New(slog.NewJSONHandler(&logs, nil)))
			server := httptest.NewServer(handler)
			defer server.Close()
			request, err := http.NewRequest(tc.method, server.URL+"/oauth2/token", nil)
			require.NoError(t, err)
			response, err := server.Client().Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			assert.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
			assert.Equal(t, tc.method, response.Header.Get("Observed-Method"))
			assert.Contains(t, logs.String(), `"method":"`+tc.observed+`"`)
			assert.NotContains(t, logs.String(), "METHOD_SECRET_SENTINEL")
			spans := recorder.Ended()
			require.Len(t, spans, 1)
			assert.True(t, strings.HasPrefix(spans[0].Name(), tc.observed+" "))
			assert.NotContains(t, fmt.Sprint(spans[0].Name(), spans[0].Attributes(), spans[0].Events(), spans[0].Status()), "METHOD_SECRET_SENTINEL")
		})
	}
}
