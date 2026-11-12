// Package livecheck sends the telemetry of a test suite to `weaver registry live-check`.
package livecheck

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// EndpointEnv names the OTLP gRPC endpoint of `weaver registry live-check`, as a URL
// such as http://127.0.0.1:14317. The CI jobs that run the live check set it.
const EndpointEnv = "TELEMETRY_LIVE_CHECK_ENDPOINT"

// The providers that export to Weaver. They are nil when the live check is off.
var (
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
)

// Run runs the tests of a suite and returns the exit code. When EndpointEnv is set, the
// global tracer and meter providers first export to that endpoint, so that Weaver can
// compare the telemetry of the suite with telemetry/registry. A test that sets its own
// global provider keeps its spans and metrics to itself.
func Run(m *testing.M) int {
	endpoint := os.Getenv(EndpointEnv)
	if endpoint == "" {
		return m.Run()
	}
	ctx := context.Background()
	traces, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpointURL(endpoint))
	if err != nil {
		fmt.Fprintln(os.Stderr, "live check: trace exporter:", err)
		return 1
	}
	metrics, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpointURL(endpoint))
	if err != nil {
		fmt.Fprintln(os.Stderr, "live check: metric exporter:", err)
		return 1
	}
	// The registry does not describe resource attributes, so the resource is empty.
	// Each span is exported when it ends, so that a burst of spans cannot overflow a queue.
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(moduleSpans{traces}),
		sdktrace.WithResource(resource.Empty()),
	)
	meterProvider = sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(moduleMetrics{metrics})),
		sdkmetric.WithResource(resource.Empty()),
	)
	Restore()

	code := m.Run()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := tracerProvider.Shutdown(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "live check: flush traces:", err)
		code = 1
	}
	if err := meterProvider.Shutdown(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "live check: flush metrics:", err)
		code = 1
	}
	return code
}

// Restore makes the providers of the live check the global providers again. A Ginkgo
// suite calls it before each spec, because a spec can leave its own provider behind. It
// does nothing when the live check is off.
func Restore() {
	if tracerProvider == nil {
		return
	}
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
}

// moduleScope reports whether an instrumentation scope belongs to this module. The module
// names a scope with one word (coding-standards/OPENTELEMETRY.md section 2). A library,
// such as the Docker client of the test containers, names its scope with an import path.
// The registry does not list the telemetry of libraries.
func moduleScope(name string) bool {
	return !strings.Contains(name, "/")
}

// moduleSpans exports only the spans of this module.
type moduleSpans struct{ sdktrace.SpanExporter }

func (e moduleSpans) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	var kept []sdktrace.ReadOnlySpan
	for _, s := range spans {
		if moduleScope(s.InstrumentationScope().Name) {
			kept = append(kept, s)
		}
	}
	return e.SpanExporter.ExportSpans(ctx, kept)
}

// moduleMetrics exports only the metrics of this module.
type moduleMetrics struct{ sdkmetric.Exporter }

func (e moduleMetrics) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	kept := *rm
	kept.ScopeMetrics = nil
	for _, sm := range rm.ScopeMetrics {
		if moduleScope(sm.Scope.Name) {
			kept.ScopeMetrics = append(kept.ScopeMetrics, sm)
		}
	}
	return e.Exporter.Export(ctx, &kept)
}
