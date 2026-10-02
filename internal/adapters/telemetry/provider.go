package telemetry

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	runtimemetrics "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/contrib/propagators/ot"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otlploghttp "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	otlpmetricgrpc "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otlpmetrichttp "go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	otlptracehttp "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/credentials"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// runtimeMetricsOnce ensures the runtime metrics goroutine is started at most once per process.
// The contrib/instrumentation/runtime package provides no stop API, so we guard against
// goroutine leaks when NewProvider is called multiple times (e.g. in tests).
var runtimeMetricsOnce sync.Once

// NewProvider initializes OTel TracerProvider, MeterProvider, and LoggerProvider
// from the given TelemetryConfig. Registers each as the global OTel provider.
// Returns a composite shutdown function that must be called on graceful shutdown.
//
// When cfg.Enabled=false, returns immediately with a no-op shutdown and nil error.
// No OTel SDK instances are created in the disabled path.
//
// When cfg.Traces.Enabled=false, the trace pipeline is skipped entirely and the
// global TracerProvider remains the SDK default (no-op). Propagators are still
// registered so that inbound trace context is propagated transparently.
//
// When cfg.Metrics.Enabled=false, the metric pipeline is skipped entirely and the
// global MeterProvider remains the SDK default (no-op).
//
// When cfg.Logs.Enabled=false, the OTLP log pipeline is skipped entirely.
// Set this to false when the collector does not support the LogsService.
//
// Supported protocols: "grpc", "http", "https". The "https" protocol uses the
// HTTP/protobuf exporter with enforced TLS (endpoint auto-prefixed with https://).
func NewProvider(ctx context.Context, cfg ports.TelemetryConfig, logger *slog.Logger) (func(context.Context) error, error) {
	if !cfg.Enabled {
		return NoopShutdown, nil
	}

	if cfg.Exporter.Protocol != ports.OTLPProtocolGRPC &&
		cfg.Exporter.Protocol != ports.OTLPProtocolHTTP &&
		cfg.Exporter.Protocol != ports.OTLPProtocolHTTPS {
		return nil, fmt.Errorf("unsupported telemetry exporter protocol: %q (expected grpc, http, or https)", cfg.Exporter.Protocol)
	}

	res, err := buildResource(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to build resource: %w", err)
	}

	// Warn if TLS is disabled (SR-003: security-first, fail closed).
	if cfg.Exporter.Insecure {
		switch cfg.Exporter.Protocol {
		case ports.OTLPProtocolHTTPS:
			logger.Warn("telemetry: insecure=true is contradictory with protocol=https and will be ignored",
				"endpoint", cfg.Exporter.Endpoint)
		case ports.OTLPProtocolHTTP:
			logger.Warn("telemetry: insecure=true has no effect for protocol=http — TLS is controlled by the URL scheme",
				"endpoint", cfg.Exporter.Endpoint)
		case ports.OTLPProtocolGRPC:
			logger.Warn("telemetry: TLS disabled for OTLP gRPC exporter (insecure=true) — do not use in production",
				"endpoint", cfg.Exporter.Endpoint)
		}
	}

	var tp *sdktrace.TracerProvider
	var mp *sdkmetric.MeterProvider
	var lp *sdklog.LoggerProvider

	switch cfg.Exporter.Protocol {
	case ports.OTLPProtocolGRPC:
		tp, mp, lp, err = buildGRPCProviders(ctx, cfg, res, logger)
	case ports.OTLPProtocolHTTP:
		tp, mp, lp, err = buildHTTPProviders(ctx, cfg, res, logger)
	case ports.OTLPProtocolHTTPS:
		// Normalize bare host:port to https:// URL for the HTTP exporter.
		if !strings.Contains(cfg.Exporter.Endpoint, "://") {
			cfg.Exporter.Endpoint = "https://" + cfg.Exporter.Endpoint
		}
		tp, mp, lp, err = buildHTTPProviders(ctx, cfg, res, logger)
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to build providers: %w", err)
	}

	// Register trace provider only when traces are enabled. When disabled, the global
	// TracerProvider remains the SDK default (no-op) so instrumented libraries produce no spans.
	if cfg.Traces.Enabled && tp != nil {
		otel.SetTracerProvider(tp)
	}

	// Register propagators unconditionally when telemetry is enabled.
	// Propagation and tracing are separate concerns: trace context must always be
	// extracted from inbound requests and injected into outbound requests so that
	// this service is transparent to distributed tracing, even when its own trace
	// pipeline is disabled.
	registerPropagators(cfg.Traces.Propagators, logger)

	// Register metric provider only when metrics are enabled. When disabled, the global
	// MeterProvider remains the SDK default (no-op) so otelchi does not export metrics.
	if cfg.Metrics.Enabled && mp != nil {
		otel.SetMeterProvider(mp)
		// The runtime metrics goroutine has no stop API; guard with Once to prevent
		// goroutine leaks when NewProvider is called multiple times (e.g. in tests).
		runtimeMetricsOnce.Do(func() {
			if err := runtimemetrics.Start(runtimemetrics.WithMinimumReadMemStatsInterval(15 * time.Second)); err != nil {
				logger.Warn("telemetry: failed to start runtime metrics", "error", err)
			}
		})
	}

	// Register log provider only when successfully initialised (log pipeline is beta).
	if lp != nil {
		global.SetLoggerProvider(lp)
	}

	return buildShutdown(tp, mp, lp), nil
}

// buildResource constructs the OTel resource with service metadata and additional
// resource attributes from config. WithAttributes is listed last so custom values
// take precedence over detected attributes on duplicate keys.
func buildResource(ctx context.Context, cfg ports.TelemetryConfig) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(cfg.ServiceName),
	}
	for k, v := range cfg.ResourceAttributes {
		attrs = append(attrs, attribute.String(k, v))
	}

	return resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithHost(),
		resource.WithProcess(),
		resource.WithAttributes(attrs...),
	)
}

// buildGRPCProviders creates TracerProvider, and optionally MeterProvider and LoggerProvider,
// using gRPC OTLP exporters.
//
// The trace exporter is only created when cfg.Traces.Enabled=true; otherwise tp is nil.
// The metric exporter is only created when cfg.Metrics.Enabled=true; otherwise mp is nil.
// The log exporter failure is non-fatal: a warning is logged and lp is nil so the caller
// skips global log provider registration.
func buildGRPCProviders(ctx context.Context, cfg ports.TelemetryConfig, res *resource.Resource, logger *slog.Logger) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *sdklog.LoggerProvider, error) {
	insecure := cfg.Exporter.Insecure

	// tlsCreds is reused across all three gRPC exporters.
	// Explicit TLS credentials make the security posture auditable rather than relying
	// on the SDK's implicit default (Principle I — Security-First, SR-003).
	tlsCreds := credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}) //nolint:gosec // TLS 1.2 minimum is intentional

	// Trace exporter — skipped when traces are disabled
	var tp *sdktrace.TracerProvider
	if cfg.Traces.Enabled {
		traceOpts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Exporter.Endpoint),
			otlptracegrpc.WithTimeout(cfg.Exporter.Timeout),
			otlptracegrpc.WithHeaders(cfg.Exporter.Headers),
		}
		if insecure {
			traceOpts = append(traceOpts, otlptracegrpc.WithInsecure())
		} else {
			traceOpts = append(traceOpts, otlptracegrpc.WithTLSCredentials(tlsCreds))
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			traceOpts = append(traceOpts, otlptracegrpc.WithCompressor("gzip"))
		}
		traceExp, err := otlptracegrpc.New(ctx, traceOpts...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("trace grpc exporter: %w", err)
		}
		tp = buildTracerProvider(traceExp, res, cfg)
	}

	// Metric exporter — skipped when metrics are disabled
	var mp *sdkmetric.MeterProvider
	if cfg.Metrics.Enabled {
		metricOpts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithEndpoint(cfg.Exporter.Endpoint),
			otlpmetricgrpc.WithTimeout(cfg.Exporter.Timeout),
			otlpmetricgrpc.WithHeaders(cfg.Exporter.Headers),
		}
		if insecure {
			metricOpts = append(metricOpts, otlpmetricgrpc.WithInsecure())
		} else {
			metricOpts = append(metricOpts, otlpmetricgrpc.WithTLSCredentials(tlsCreds))
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			metricOpts = append(metricOpts, otlpmetricgrpc.WithCompressor("gzip"))
		}
		metricExp, err := otlpmetricgrpc.New(ctx, metricOpts...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("metric grpc exporter: %w", err)
		}
		mp = buildMeterProvider(metricExp, res, cfg)
	}

	// Log exporter — skipped when logs are disabled, non-fatal when enabled:
	// failure is logged as a warning and lp is nil.
	var lp *sdklog.LoggerProvider
	if cfg.Logs.Enabled {
		logExp, logErr := buildLogExporter(ctx, cfg, tlsCreds)
		if logErr != nil {
			logger.Warn("telemetry: log gRPC exporter failed to initialize, OTLP log pipeline disabled", "error", logErr)
		} else {
			lp = buildLoggerProvider(logExp, res)
		}
	}

	return tp, mp, lp, nil
}

// buildHTTPProviders creates TracerProvider, and optionally MeterProvider and LoggerProvider,
// using HTTP OTLP exporters.
//
// The endpoint must be a full URL (e.g. "https://collector:4318" or "http://localhost:4318").
// WithEndpointURL is used so the URL scheme controls TLS; WithInsecure is not needed.
//
// The trace exporter is only created when cfg.Traces.Enabled=true; otherwise tp is nil.
// The metric exporter is only created when cfg.Metrics.Enabled=true; otherwise mp is nil.
// The log exporter failure is non-fatal: a warning is logged and lp is nil.
func buildHTTPProviders(ctx context.Context, cfg ports.TelemetryConfig, res *resource.Resource, logger *slog.Logger) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider, *sdklog.LoggerProvider, error) {
	// Trace exporter — skipped when traces are disabled; endpoint must be a full http(s) URL
	var tp *sdktrace.TracerProvider
	if cfg.Traces.Enabled {
		traceOpts := []otlptracehttp.Option{
			otlptracehttp.WithEndpointURL(cfg.Exporter.Endpoint),
			otlptracehttp.WithTimeout(cfg.Exporter.Timeout),
			otlptracehttp.WithHeaders(cfg.Exporter.Headers),
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			traceOpts = append(traceOpts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}
		traceExp, err := otlptracehttp.New(ctx, traceOpts...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("trace http exporter: %w", err)
		}
		tp = buildTracerProvider(traceExp, res, cfg)
	}

	// Metric exporter — skipped when metrics are disabled
	var mp *sdkmetric.MeterProvider
	if cfg.Metrics.Enabled {
		metricOpts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithEndpointURL(cfg.Exporter.Endpoint),
			otlpmetrichttp.WithTimeout(cfg.Exporter.Timeout),
			otlpmetrichttp.WithHeaders(cfg.Exporter.Headers),
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			metricOpts = append(metricOpts, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
		}
		metricExp, err := otlpmetrichttp.New(ctx, metricOpts...)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("metric http exporter: %w", err)
		}
		mp = buildMeterProvider(metricExp, res, cfg)
	}

	// Log exporter — skipped when logs are disabled, non-fatal when enabled:
	// failure is logged as a warning and lp is nil.
	var lp *sdklog.LoggerProvider
	if cfg.Logs.Enabled {
		logExp, logErr := buildLogExporter(ctx, cfg, nil)
		if logErr != nil {
			logger.Warn("telemetry: log HTTP exporter failed to initialize, OTLP log pipeline disabled", "error", logErr)
		} else {
			lp = buildLoggerProvider(logExp, res)
		}
	}

	return tp, mp, lp, nil
}

func buildLogExporter(ctx context.Context, cfg ports.TelemetryConfig, tlsCreds credentials.TransportCredentials) (sdklog.Exporter, error) {
	switch cfg.Exporter.Protocol {
	case ports.OTLPProtocolGRPC:
		options := []otlploggrpc.Option{
			otlploggrpc.WithEndpoint(cfg.Exporter.Endpoint),
			otlploggrpc.WithTimeout(cfg.Exporter.Timeout),
			otlploggrpc.WithHeaders(cfg.Exporter.Headers),
		}
		if cfg.Exporter.Insecure {
			options = append(options, otlploggrpc.WithInsecure())
		} else {
			if tlsCreds == nil {
				tlsCreds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}) //nolint:gosec // TLS 1.2 minimum matches the existing exporters.
			}
			options = append(options, otlploggrpc.WithTLSCredentials(tlsCreds))
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			options = append(options, otlploggrpc.WithCompressor("gzip"))
		}
		return otlploggrpc.New(ctx, options...)
	case ports.OTLPProtocolHTTP, ports.OTLPProtocolHTTPS:
		endpoint := cfg.Exporter.Endpoint
		if cfg.Exporter.Protocol == ports.OTLPProtocolHTTPS && !strings.Contains(endpoint, "://") {
			endpoint = "https://" + endpoint
		}
		options := []otlploghttp.Option{
			otlploghttp.WithEndpointURL(endpoint),
			otlploghttp.WithTimeout(cfg.Exporter.Timeout),
			otlploghttp.WithHeaders(cfg.Exporter.Headers),
		}
		if cfg.Exporter.Compression == ports.OTLPCompressionGzip {
			options = append(options, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		}
		return otlploghttp.New(ctx, options...)
	default:
		return nil, fmt.Errorf("unsupported telemetry exporter protocol")
	}
}

// buildTracerProvider constructs a TracerProvider with sampling and batching.
func buildTracerProvider(exp sdktrace.SpanExporter, res *resource.Resource, cfg ports.TelemetryConfig) *sdktrace.TracerProvider {
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.Traces.SamplingRate))
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
}

// registerPropagators sets up the global TextMapPropagator from config.
// Supported propagator names:
//   - "tracecontext" — W3C Trace Context (traceparent/tracestate headers)
//   - "baggage"      — W3C Baggage
//   - "b3multi"      — Zipkin B3 Multiple Headers (X-B3-TraceId, X-B3-SpanId, …)
//   - "b3"           — Zipkin B3 Single Header (b3)
//   - "ottrace"      — OpenTracing (ot-tracer-*) for OT↔OTel interoperability
//
// Unrecognized propagator names are logged as warnings and skipped.
func registerPropagators(propagatorNames []string, logger *slog.Logger) {
	var propagators []propagation.TextMapPropagator
	for _, name := range propagatorNames {
		switch name {
		case "tracecontext":
			propagators = append(propagators, propagation.TraceContext{})
		case "baggage":
			propagators = append(propagators, propagation.Baggage{})
		case "b3multi":
			propagators = append(propagators, b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader)))
		case "b3":
			propagators = append(propagators, b3.New())
		case "ottrace":
			propagators = append(propagators, ot.OT{})
		default:
			logger.Warn("telemetry: unrecognized propagator name, ignoring", "propagator", name)
		}
	}
	if len(propagators) > 0 {
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagators...))
	}
}

// buildMeterProvider constructs a MeterProvider with a periodic reader.
func buildMeterProvider(exp sdkmetric.Exporter, res *resource.Resource, cfg ports.TelemetryConfig) *sdkmetric.MeterProvider {
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exp,
			sdkmetric.WithInterval(cfg.Metrics.ExportInterval),
		)),
		sdkmetric.WithResource(res),
	)
}

// buildLoggerProvider constructs a LoggerProvider with a batch processor.
func buildLoggerProvider(exp sdklog.Exporter, res *resource.Resource) *sdklog.LoggerProvider {
	return sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		sdklog.WithResource(res),
	)
}

// buildShutdown creates a composite shutdown function that shuts down all
// initialised providers concurrently and collects any errors.
// tp, mp, and lp may be nil when those pipelines were disabled or failed to initialise.
func buildShutdown(tp *sdktrace.TracerProvider, mp *sdkmetric.MeterProvider, lp *sdklog.LoggerProvider) func(context.Context) error {
	return func(ctx context.Context) error {
		var g errgroup.Group
		if tp != nil {
			g.Go(func() error {
				if err := tp.Shutdown(ctx); err != nil {
					return fmt.Errorf("tracer shutdown: %w", err)
				}
				return nil
			})
		}
		if mp != nil {
			g.Go(func() error {
				if err := mp.Shutdown(ctx); err != nil {
					return fmt.Errorf("meter shutdown: %w", err)
				}
				return nil
			})
		}
		if lp != nil {
			g.Go(func() error {
				if err := lp.Shutdown(ctx); err != nil {
					return fmt.Errorf("logger shutdown: %w", err)
				}
				return nil
			})
		}
		return g.Wait()
	}
}
