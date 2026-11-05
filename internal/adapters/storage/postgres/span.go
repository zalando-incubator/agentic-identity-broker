package postgres

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// storageOutcomeCanceled is the storage.outcome value for caller cancellation. The other values
// are the storage.ErrorKind of the result.
const storageOutcomeCanceled = "canceled"

// startStorageSpan starts the Client span of a repository method. The returned function records
// the method's result and ends the span.
func startStorageSpan(ctx context.Context, name, operation string) (context.Context, func(err error)) {
	ctx, span := otel.Tracer("storage").Start(ctx, name,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			semconv.DBSystemKey.String("postgresql"),
			attribute.String("db.operation", operation),
		),
	)
	return ctx, func(err error) {
		recordStorageResult(span, err)
		span.End()
	}
}

// recordStorageResult sets the error status only when the database failed. A missing row, a
// conflict, rejected input and caller cancellation are results the caller handles, so they are
// recorded as storage.outcome. The cause is not recorded because it can contain SQL or a
// connection string.
func recordStorageResult(span trace.Span, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, context.Canceled) {
		span.SetAttributes(attribute.String("storage.outcome", storageOutcomeCanceled))
		return
	}
	kind := storage.ErrorKindUnknown
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		kind = storageErr.Kind
	}
	switch kind {
	case storage.ErrorKindNotFound, storage.ErrorKindConflict, storage.ErrorKindValidation:
		span.SetAttributes(attribute.String("storage.outcome", string(kind)))
	default:
		span.SetAttributes(attribute.String("error.type", string(kind)))
		span.SetStatus(codes.Error, "storage operation failed")
	}
}
