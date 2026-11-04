package delegation

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func observeResolution(span trace.Span, err error) {
	outcome, detail := "success", "none"
	var denied *DeniedError
	switch {
	case err == nil:
	case errors.Is(err, context.Canceled):
		outcome, detail = "canceled", "caller_canceled"
	case errors.As(err, &denied):
		outcome, detail = "authorization_denied", "delegation_denied"
	default:
		outcome, detail = "infrastructure_error", "delegation_unavailable"
		category := CategoryUnavailable
		var resolutionErr *Error
		if errors.As(err, &resolutionErr) {
			switch resolutionErr.Category {
			case CategoryTimeout, CategoryUnavailable, CategoryRejected, CategoryInvalidResponse:
				category = resolutionErr.Category
			}
		}
		span.SetAttributes(attribute.String("error.type", category))
		span.SetStatus(codes.Error, "delegation context unavailable")
	}
	span.SetAttributes(
		attribute.String("token_exchange.operation", "delegation_resolution"),
		attribute.String("token_exchange.outcome", outcome),
		attribute.String("token_exchange.failure_detail", detail),
	)
}
