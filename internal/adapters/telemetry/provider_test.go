package telemetry

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/log/global"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// saveAndRestoreGlobalProviders saves the current global OTel providers and schedules their
// restoration via t.Cleanup. Call this at the beginning of any test that invokes NewProvider
// to prevent global-state leakage across parallel test packages.
func saveAndRestoreGlobalProviders(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevLP := global.GetLoggerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		global.SetLoggerProvider(prevLP)
	})
}

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}

func minimalEnabledConfig(protocol string) ports.TelemetryConfig {
	cfg := ports.DefaultTelemetryConfig()
	cfg.Enabled = true
	cfg.Exporter.Protocol = protocol
	cfg.Exporter.Endpoint = "localhost:4317"
	cfg.Exporter.Insecure = true
	cfg.ServiceName = "test-service"
	return cfg
}

// Disabled fast-path — NewProvider with Enabled=false returns noop shutdown with no error.
func TestNewProvider_DisabledReturnsNoop(t *testing.T) {
	cfg := ports.DefaultTelemetryConfig()
	cfg.Enabled = false

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown, "shutdown function must not be nil")

	// Calling shutdown on the noop provider must return nil
	require.NoError(t, shutdown(ctx))
}

// Invalid protocol returns non-nil error.
func TestNewProvider_InvalidProtocol(t *testing.T) {
	cfg := ports.DefaultTelemetryConfig()
	cfg.Enabled = true
	cfg.Exporter.Protocol = "jaeger"
	cfg.Exporter.Endpoint = "localhost:4317"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	_, err := NewProvider(ctx, cfg, logger)
	require.Error(t, err, "unsupported protocol must return an error")
	assert.Contains(t, err.Error(), "jaeger")
}

// HTTPS protocol initializes using HTTP exporter with auto-prefixed endpoint.
func TestNewProvider_HTTPSInitializes(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("https")
	// Bare host:port — provider should auto-prefix https://
	cfg.Exporter.Endpoint = "localhost:4318"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "HTTPS provider must initialize with bare host:port")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// HTTPS protocol with full https:// URL initializes.
func TestNewProvider_HTTPSWithFullURL(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("https")
	cfg.Exporter.Endpoint = "https://localhost:4318"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "HTTPS provider must initialize with full https:// URL")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// HTTPS + insecure=true logs a warning about the contradiction.
func TestNewProvider_HTTPSInsecureWarning(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("https")
	cfg.Exporter.Endpoint = "localhost:4318"
	cfg.Exporter.Insecure = true

	ctx := context.Background()
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(cancelCtx)
	})

	output := buf.String()
	assert.Contains(t, output, "contradictory",
		"expected warning about insecure+https contradiction, got: %s", output)
}

// Gzip compression option is accepted without error for gRPC.
func TestNewProvider_GRPCWithGzipCompression(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("grpc")
	cfg.Exporter.Compression = "gzip"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "gRPC with gzip compression must initialize")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// Gzip compression option is accepted without error for HTTP.
func TestNewProvider_HTTPWithGzipCompression(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("http")
	cfg.Exporter.Endpoint = "http://localhost:4318"
	cfg.Exporter.Compression = "gzip"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "HTTP with gzip compression must initialize")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// Grpc + insecure=true logs a "TLS disabled" warning (insecure flag actually changes behavior).
func TestNewProvider_InsecureLogsWarning(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("grpc")
	cfg.Exporter.Insecure = true

	ctx := context.Background()
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(cancelCtx)
	})

	output := buf.String()
	assert.Contains(t, output, "TLS disabled",
		"expected gRPC insecure warning to mention TLS disabled, got: %s", output)
}

// Http + insecure=true logs a warning that insecure has no effect (TLS is URL-scheme-controlled).
func TestNewProvider_HTTPInsecureIgnoredWarning(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("http")
	cfg.Exporter.Insecure = true

	ctx := context.Background()
	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(cancelCtx)
	})

	output := buf.String()
	assert.Contains(t, output, "no effect",
		"expected http insecure warning to say insecure has no effect, got: %s", output)
	assert.NotContains(t, output, "TLS disabled",
		"expected no 'TLS disabled' warning for protocol=http, got: %s", output)
}

// gRPC provider initializes without a real collector.
// The OTel SDK buffers telemetry and does not fail on initialization when no
// collector is reachable. Shutdown may return an error on flush — that's acceptable.
func TestNewProvider_GRPCInitializes(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("grpc")

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "gRPC provider must initialize without a real collector")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		// Use a cancelled context so shutdown returns immediately without blocking on export
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// HTTP provider initializes without a real collector.
// The OTel SDK buffers telemetry and does not fail on initialization when no
// collector is reachable. Shutdown may return an error on flush — that's acceptable.
func TestNewProvider_HTTPInitializes(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("http")
	// HTTP endpoint must be a full URL (WithEndpointURL is used in buildHTTPProviders)
	cfg.Exporter.Endpoint = "http://localhost:4318"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err, "HTTP provider must initialize without a real collector")
	require.NotNil(t, shutdown)

	t.Cleanup(func() {
		// Use a cancelled context so shutdown returns immediately without blocking on export
		shutdownCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(shutdownCtx)
	})
}

// Custom service name appears in TracerProvider resource via emitted spans.
func TestNewProvider_CustomServiceName(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	cfg := minimalEnabledConfig("grpc")
	cfg.ServiceName = "my-service"

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()
		_ = shutdown(cancelCtx)
	})

	// The global TracerProvider should now be a real SDK provider
	tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "global TracerProvider must be *sdktrace.TracerProvider after NewProvider")

	// Register an in-memory recorder to capture a span and inspect its resource
	recorder := tracetest.NewSpanRecorder()
	tp.RegisterSpanProcessor(recorder)

	// Emit a span to capture resource metadata
	tracer := otel.GetTracerProvider().Tracer("test")
	spanCtx, span := tracer.Start(ctx, "test-span")
	span.End()
	_ = spanCtx

	ended := recorder.Ended()
	require.NotEmpty(t, ended, "expected at least one ended span")

	res := ended[0].Resource()
	require.NotNil(t, res)

	found := false
	for _, attr := range res.Attributes() {
		if attr.Key == semconv.ServiceNameKey && attr.Value.AsString() == "my-service" {
			found = true
			break
		}
	}
	assert.True(t, found, "resource must contain service.name=my-service")
}

func TestNewProvider_HTTPDefaultPathRegression(t *testing.T) {
	saveAndRestoreGlobalProviders(t)

	var mu sync.Mutex
	var gotPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotPaths = append(gotPaths, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := minimalEnabledConfig("http")
	cfg.Exporter.Endpoint = server.URL
	cfg.Exporter.Timeout = 5 * time.Second

	ctx := context.Background()
	logger := newTestLogger(new(bytes.Buffer))

	shutdown, err := NewProvider(ctx, cfg, logger)
	require.NoError(t, err)
	require.NotNil(t, shutdown)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = shutdown(shutdownCtx)
	})

	tp, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider)
	require.True(t, ok, "global TracerProvider must be *sdktrace.TracerProvider after NewProvider")

	_, span := tp.Tracer("test").Start(ctx, "test-span")
	span.End()

	flushCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, tp.ForceFlush(flushCtx))

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, gotPaths, "expected the span export request to reach the test server")
	assert.Equal(t, "/v1/traces", gotPaths[0],
		"bare host:port endpoint must default to the OTLP standard traces path, not root")
}

func TestRegisterPropagators_AllSupported(t *testing.T) {
	// Save and restore global propagator to avoid leaking state.
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	// Register all supported propagators.
	registerPropagators([]string{"tracecontext", "baggage", "b3multi", "b3", "ottrace"}, logger)

	// Verify composite propagator reports the expected header fields.
	prop := otel.GetTextMapPropagator()
	fields := prop.Fields()
	assert.NotEmpty(t, fields, "composite propagator must report header fields")

	// No warnings should have been logged.
	assert.Empty(t, buf.String(), "no warnings expected for valid propagator names")
}

func TestRegisterPropagators_UnknownSkipped(t *testing.T) {
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })

	var buf bytes.Buffer
	logger := newTestLogger(&buf)

	registerPropagators([]string{"b3multi", "nonexistent"}, logger)

	assert.Contains(t, buf.String(), "nonexistent", "should warn about unrecognized propagator")
}
