// Package fixture has one example of each pattern that telemetrydoc resolves.
package fixture

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Outcome is an enumerated attribute value.
type Outcome string

const (
	// OutcomeSuccess means the operation worked.
	OutcomeSuccess Outcome = "success"
	// OutcomeFailure means the operation failed.
	OutcomeFailure Outcome = "failure" // #nosec G101 -- not a credential.
)

// SpanAttribute returns o under key, as id.Principal.SpanAttribute does.
func (o Outcome) SpanAttribute(key string) attribute.KeyValue {
	return attribute.String(key, string(o))
}

const spanFetch = "fixture.fetch"

var tracer = otel.Tracer("fixture")

var outcomeKey = attribute.Key("fixture.outcome")

// Direct start: constant name, kind, attributes at start and later, error status.
func Fetch(ctx context.Context, n int) error {
	ctx, span := tracer.Start(ctx, spanFetch,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system", "postgresql")))
	defer span.End()
	cache := "cold"
	if n > 0 {
		cache = "warm"
	}
	span.SetAttributes(attribute.Int("fixture.count", n), attribute.String("fixture.cache", cache),
		OutcomeSuccess.SpanAttribute("fixture.result"))
	annotate(context.WithoutCancel(ctx), OutcomeFailure)
	span.SetStatus(codes.Error, "fetch failed")
	return nil
}

// annotate sets an attribute on the span in the context.
func annotate(ctx context.Context, o Outcome) {
	trace.SpanFromContext(ctx).SetAttributes(outcomeKey.String(string(o)))
}

// startOp is a wrapper that receives the span name and an attribute value.
func startOp(ctx context.Context, name, op string) (context.Context, trace.Span) {
	return otel.Tracer("fixture").Start(ctx, name, trace.WithAttributes(
		attribute.Bool("fixture.wrapped", true), attribute.String("fixture.op", op)))
}

func Wrapped(ctx context.Context) {
	_, span := startOp(ctx, "fixture.wrapped_op", "WrappedOp")
	defer span.End()
	span.SetAttributes(attribute.String("fixture.mode", "fast"))
}

func Renamed(ctx context.Context, special bool) {
	_, span := tracer.Start(ctx, "fixture.generic")
	defer span.End()
	if special {
		span.SetName("fixture.special")
	}
}

// holder keeps a span in a struct field and finishes it later.
type holder struct{ span trace.Span }

func Held(ctx context.Context) *holder {
	_, span := tracer.Start(ctx, "fixture.held")
	return &holder{span: span}
}

// Reopen starts a span straight into the field.
func (h *holder) Reopen(ctx context.Context) {
	_, h.span = tracer.Start(ctx, "fixture.reopened")
}

func (h *holder) Close() { h.finish("close_failed") }

func (h *holder) finish(kind string) {
	h.span.SetAttributes(attribute.String("error.type", kind))
	h.span.SetStatus(codes.Error, "held failed")
	h.span.End()
}

// Abort passes the held span to a function that another span also reaches.
func (h *holder) Abort() { fail(h.span, "abort_failed") }

// fail sets the error of the span that it receives.
func fail(span trace.Span, kind string) {
	span.SetAttributes(attribute.String("error.type", kind))
	span.SetStatus(codes.Error, "failed")
}

// Direct passes its own span to fail.
func Direct(ctx context.Context) {
	_, span := tracer.Start(ctx, "fixture.direct")
	defer span.End()
	fail(span, "direct_failed")
}

// labeler is an interface, so the callers of its method are not all known.
type labeler interface {
	label(ctx context.Context, value string)
}

type staticLabeler struct{}

func (staticLabeler) label(ctx context.Context, value string) {
	_, span := tracer.Start(ctx, "fixture.labeled")
	defer span.End()
	span.SetAttributes(attribute.String("fixture.label", value))
}

func Labeled(ctx context.Context, l labeler, dynamic string) {
	staticLabeler{}.label(ctx, "static")
	l.label(ctx, dynamic)
}

type meters struct {
	requests metric.Int64Counter
	source   attribute.KeyValue
}

func newMeters() *meters {
	m := otel.Meter("fixture")
	requests, _ := m.Int64Counter("fixture.requests",
		metric.WithDescription("Requests handled"), metric.WithUnit("{request}"))
	queue := func(name string) attribute.KeyValue { return attribute.String("queue", name) }
	_, _ = m.Int64ObservableGauge("fixture.queue_depth",
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(1, metric.WithAttributes(queue("main")))
			return nil
		}))
	return &meters{requests: requests, source: attribute.String("source", "fixture")}
}

func (m *meters) record(ctx context.Context, o Outcome) {
	m.requests.Add(ctx, 1, metric.WithAttributes(m.source, attribute.String("outcome", string(o))))
}
