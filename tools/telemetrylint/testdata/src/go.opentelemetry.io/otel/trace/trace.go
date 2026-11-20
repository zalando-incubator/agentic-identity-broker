// Package trace is a minimal stub of go.opentelemetry.io/otel/trace for analyzer tests.
package trace

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type SpanKind int

const (
	SpanKindInternal SpanKind = 1
	SpanKindServer   SpanKind = 2
	SpanKindClient   SpanKind = 3
)

type SpanStartOption interface{ applySpanStart() }

type EventOption interface{ applyEvent() }

type TracerOption interface{ applyTracer() }

type option struct{}

func (option) applySpanStart() {}
func (option) applyEvent()     {}

type Link struct{}

func WithSpanKind(kind SpanKind) SpanStartOption { return option{} }

func WithNewRoot() SpanStartOption { return option{} }

func WithLinks(links ...Link) SpanStartOption { return option{} }

func WithAttributes(attrs ...attribute.KeyValue) SpanStartOption { return option{} }

type Span interface {
	End()
	SetName(name string)
	AddEvent(name string, options ...EventOption)
	SetStatus(code codes.Code, description string)
	SetAttributes(kv ...attribute.KeyValue)
}

type Tracer interface {
	Start(ctx context.Context, spanName string, opts ...SpanStartOption) (context.Context, Span)
}

type TracerProvider interface {
	Tracer(name string, options ...TracerOption) Tracer
}
