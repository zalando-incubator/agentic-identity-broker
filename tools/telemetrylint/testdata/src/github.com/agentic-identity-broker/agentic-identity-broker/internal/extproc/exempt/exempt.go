// Package exempt stands for ExtProc, which must not import id.Principal (ADR 027) and sets the
// raw principal under a reserved key.
package exempt

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func setPrincipal(span trace.Span, principal string) {
	span.SetAttributes(attribute.String("approval.principal", principal))
}
