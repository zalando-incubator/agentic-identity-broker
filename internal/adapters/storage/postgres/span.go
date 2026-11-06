package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
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
		recordStorageResult(ctx, span, err)
		span.End()
	}
}

// recordStorageResult sets the error status only when the operation failed. A missing row, a
// conflict, rejected input and caller cancellation are results the caller handles, so they are
// recorded as storage.outcome. The cause is not recorded because it can contain SQL or a connection
// string.
func recordStorageResult(ctx context.Context, span trace.Span, err error) {
	if err == nil {
		return
	}
	if canceledByCaller(ctx, err) {
		span.SetAttributes(attribute.String("storage.outcome", storageOutcomeCanceled))
		return
	}
	kind := storage.ErrorKindUnknown
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		kind = storageErr.Kind
	}
	if isStatementTimeout(err) {
		kind = storage.ErrorKindTimeout
	}
	switch kind {
	case storage.ErrorKindNotFound, storage.ErrorKindConflict, storage.ErrorKindValidation:
		span.SetAttributes(attribute.String("storage.outcome", string(kind)))
	default:
		span.SetAttributes(attribute.String("error.type", string(kind)))
		span.SetStatus(codes.Error, "storage operation failed")
	}
}

// canceledByCaller reports whether err comes from the caller canceling the operation. A transaction
// can hide the cancellation: a statement on the connection that the cancellation closed returns
// driver.ErrBadConn, and a commit after the resulting rollback returns sql.ErrTxDone. Any other
// error after a cancellation is a real failure.
func canceledByCaller(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	return errors.Is(ctx.Err(), context.Canceled) && (errors.Is(err, driver.ErrBadConn) || errors.Is(err, sql.ErrTxDone))
}

// isStatementTimeout reports whether PostgreSQL ended the query because of statement_timeout. A
// query canceled by an operator has the same SQLSTATE, so only the message tells them apart. A
// server that translates its messages keeps reporting the repository's error kind.
func isStatementTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014" && strings.Contains(pgErr.Message, "statement timeout")
}
