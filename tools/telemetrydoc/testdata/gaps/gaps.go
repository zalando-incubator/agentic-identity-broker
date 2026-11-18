// Package gaps has each pattern that telemetrydoc reports as a gap.
package gaps

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("gaps")

func Dynamic(ctx context.Context, name, key string) {
	_, span := tracer.Start(ctx, "gaps."+name)
	defer span.End()
	_, span2 := tracer.Start(ctx, "gaps.static")
	defer span2.End()
	span2.SetAttributes(attribute.String(key, "x"))
}

// Orphan sets an attribute on a span that no static caller starts.
func Orphan(ctx context.Context) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("gaps.orphan", "x"))
}

// Unused builds an attribute that reaches no span or metric.
func Unused() {
	_ = attribute.String("gaps.unused", "x")
}

// Twice starts one span name with two kinds.
func Twice(ctx context.Context) {
	_, a := tracer.Start(ctx, "gaps.twice")
	a.End()
	_, b := tracer.Start(ctx, "gaps.twice", trace.WithSpanKind(trace.SpanKindClient))
	b.End()
}

// Kind passes the span kind in a variable.
func Kind(ctx context.Context) {
	kind := trace.SpanKindClient
	_, span := tracer.Start(ctx, "gaps.kind", trace.WithSpanKind(kind))
	span.End()
}

// Event sets attributes on a span event.
func Event(ctx context.Context) {
	_, span := tracer.Start(ctx, "gaps.event")
	defer span.End()
	span.AddEvent("retry", trace.WithAttributes(attribute.Int("gaps.attempt", 2)))
}

type finisher interface{ finish(trace.Span) }

// Passed hands the span to an interface method.
func Passed(ctx context.Context, f finisher) {
	_, span := tracer.Start(ctx, "gaps.passed")
	f.finish(span)
}

// Options receives attributes from callers that the generator cannot see.
func (S) Options(ctx context.Context, attrs ...attribute.KeyValue) {
	_, span := tracer.Start(ctx, "gaps.options", trace.WithAttributes(attrs...))
	span.End()
}

type S struct{}

// named starts a span with a named result as its name.
func named(ctx context.Context) (name string) {
	name = "gaps.named"
	_, span := tracer.Start(ctx, name)
	span.End()
	return name
}

// Collide renames a span to the name of another span.
func Collide(ctx context.Context) {
	_, span := tracer.Start(ctx, "gaps.collide")
	defer span.End()
	span.SetName("gaps.static")
}

// Scoped starts a span on a tracer whose name is not a constant.
func Scoped(ctx context.Context, scope string) {
	_, span := otel.Tracer(scope).Start(ctx, "gaps.scoped")
	span.End()
}

// Meters creates one metric twice and records on an instrument it receives.
func Meters(ctx context.Context, c metric.Int64Counter) {
	m := otel.Meter("gaps")
	_, _ = m.Int64Counter("gaps.dup")
	_, _ = m.Int64Counter("gaps.dup")
	c.Add(ctx, 1)
	_ = named(ctx)
}
