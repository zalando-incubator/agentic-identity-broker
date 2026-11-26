// Package bootstrap provides test infrastructure for E2E testing.
package bootstrap

import (
	"context"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// NewInMemoryTracerProvider creates an in-memory TracerProvider for E2E testing.
// Returns both the provider and the SpanRecorder for asserting on emitted spans.
// The provider uses AlwaysSample to capture all spans during tests.
func NewInMemoryTracerProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	return tp, recorder
}

// SpanStartSignal is a span processor that closes Started when the first span with the given
// name starts. Tests use it to act while an operation is in progress, without fixed sleeps.
type SpanStartSignal struct {
	name    string
	once    sync.Once
	Started chan struct{}
}

// NewSpanStartSignal signals the start of the named span.
func NewSpanStartSignal(name string) *SpanStartSignal {
	return &SpanStartSignal{name: name, Started: make(chan struct{})}
}

func (s *SpanStartSignal) OnStart(_ context.Context, span sdktrace.ReadWriteSpan) {
	if span.Name() == s.name {
		s.once.Do(func() { close(s.Started) })
	}
}

func (s *SpanStartSignal) OnEnd(sdktrace.ReadOnlySpan) {}

func (s *SpanStartSignal) Shutdown(context.Context) error { return nil }

func (s *SpanStartSignal) ForceFlush(context.Context) error { return nil }

// NewSignalingTracerProvider is NewInMemoryTracerProvider plus a start signal for spanName.
func NewSignalingTracerProvider(spanName string) (*sdktrace.TracerProvider, *tracetest.SpanRecorder, *SpanStartSignal) {
	recorder := tracetest.NewSpanRecorder()
	signal := NewSpanStartSignal(spanName)
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(signal),
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	return tp, recorder, signal
}
