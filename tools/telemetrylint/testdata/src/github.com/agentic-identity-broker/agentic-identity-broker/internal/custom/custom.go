package custom

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func custom(ctx context.Context) {
	_, span := otel.Tracer("billing").Start(ctx, "billing.charge", trace.WithSpanKind(trace.SpanKindInternal))
	otel.Tracer("storage")                                               // want `scope: instrumentation scope "storage" is not in the allowlist`
	span.SetAttributes(attribute.String("billing.principal", "someone")) // want `principal-key: key "billing.principal" is reserved for principals`
}
