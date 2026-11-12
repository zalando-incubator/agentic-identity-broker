package spans

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

const getAgent = "storage.get.agent"

var client = trace.WithSpanKind(trace.SpanKindClient)

// useHelpers comes before the helpers it calls: helpers are found before call sites are checked.
func useHelpers(ctx context.Context, dynamic string) {
	startLifecycleSpan(ctx, "approval.review", trace.WithSpanKind(trace.SpanKindInternal))
	startLifecycleSpan(ctx, "approval.review")        // want `span-kind: the span start does not set trace.WithSpanKind`
	startLifecycleSpan(ctx, "storage.review", client) // want `span-name: span name "storage.review" does not start with the tracer scope "approval"`
	startStorageSpan(ctx, "storage.get.agent")
	startStorageSpan(ctx, "storage.Get") // want `span-name: span name "storage.Get" does not match <scope>.<operation>\[.<entity>\]`
	startNested(ctx, "approval.sync", client)
	startNested(ctx, "approval.sync")                  // want `span-kind: the span start does not set trace.WithSpanKind`
	startNested(ctx, "approval.sync."+dynamic, client) // want `span-name: the span name must be a constant string`
}

func startLifecycleSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts, trace.WithNewRoot())
	return otel.Tracer("approval").Start(ctx, name, opts...)
}

func startStorageSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer("storage").Start(ctx, name, trace.WithSpanKind(trace.SpanKindClient))
}

func startNested(ctx context.Context, name string, opts ...trace.SpanStartOption) {
	startLifecycleSpan(ctx, name, opts...)
}

func names(ctx context.Context, dynamic string, p id.Principal) {
	tracer := otel.Tracer("storage")
	_, span := tracer.Start(ctx, getAgent, client)
	span.SetName("storage.list.agent")
	tracer.Start(ctx, "storage.Get", client)                     // want `span-name: span name "storage.Get" does not match`
	tracer.Start(ctx, "storage.get.agent.by_id", client)         // want `span-name: span name "storage.get.agent.by_id" does not match`
	tracer.Start(ctx, "approval.list", client)                   // want `span-name: span name "approval.list" does not start with the tracer scope "storage"`
	otel.Tracer("approval").Start(ctx, "billing.charge", client) // want `span-name: span name "billing.charge" does not start with a scope from OPENTELEMETRY.md section 2`
	tracer.Start(ctx, "storage."+dynamic, client)                // want `span-name: the span name must be a constant string`
	tracer.Start(ctx, p.String(), client)                        // want `principal-sink: a principal must not be in a span name`
	span.SetName(dynamic)                                        // want `span-name: the span name must be a constant string`
	span.SetName("approval for " + string(p))                    // want `principal-sink: a principal must not be in a span name`
}

func kinds(ctx context.Context, tracer trace.Tracer) {
	tracer.Start(ctx, "storage.get.agent") // want `span-kind: the span start does not set trace.WithSpanKind`
	rootOnly := []trace.SpanStartOption{trace.WithNewRoot()}
	tracer.Start(ctx, "storage.get.agent", rootOnly...) // want `span-kind: the span start does not set trace.WithSpanKind`
	withKind := []trace.SpanStartOption{trace.WithNewRoot()}
	withKind = append(withKind, trace.WithSpanKind(trace.SpanKindInternal))
	tracer.Start(ctx, "storage.get.agent", withKind...)
	go func(name string) {
		tracer.Start(ctx, name, client) // want `span-name: the span name must be a constant string`
	}("storage.get.agent")
}

func events(span trace.Span, p id.Principal) {
	span.AddEvent("approval.reviewed")
	span.AddEvent(string(p)) // want `principal-sink: a principal must not be in a span event`
	span.SetStatus(codes.Error, "approval lookup failed")
	span.SetStatus(codes.Error, "lookup failed for "+p.String()) // want `principal-sink: a principal must not be in a span status description`
}
