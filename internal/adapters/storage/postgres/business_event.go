package postgres

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Append uses the pool even when ctx carries an already committed state-change transaction.
func (a *Adapter) Append(ctx context.Context, event *model.BusinessEvent) (err error) {
	const operation = "Append"
	ctx, finish := startStorageSpan(ctx, "storage.append.business_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event cannot be nil")
	}
	if err := event.Validate(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid business event")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	if err := execCtx.Err(); err != nil {
		return businessEventStorageError(operation, err, "failed to append business event")
	}
	var attributes any = "{}"
	if event.Attributes != nil {
		attributes, err = json.Marshal(event.Attributes)
		if err != nil {
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid business event attributes")
		}
	}
	var recordedAt time.Time
	err = sqlx.GetContext(execCtx, a.db, &recordedAt, `INSERT INTO business_events
		(id, event, occurred_at, principal, actor, trace_id, attributes)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING recorded_at`, event.ID, event.Event, event.OccurredAt.UTC(),
		event.Principal, event.Actor, event.TraceID, attributes)
	if err != nil {
		return businessEventStorageError(operation, err, "failed to append business event")
	}
	event.RecordedAt = recordedAt
	return nil
}

func (a *Adapter) StorageTime(ctx context.Context) (now time.Time, err error) {
	const operation = "StorageTime"
	ctx, finish := startStorageSpan(ctx, "storage.current_time.business_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return time.Time{}, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()
	if err := sqlx.GetContext(execCtx, a.db, &now, `SELECT statement_timestamp()`); err != nil {
		return time.Time{}, businessEventStorageError(operation, err, "failed to read business event storage time")
	}
	return now, nil
}

// DeleteRecordedBefore uses one bounded statement outside any transaction carried by ctx.
func (a *Adapter) DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (deleted int64, err error) {
	const operation = "DeleteRecordedBefore"
	ctx, finish := startStorageSpan(ctx, "storage.delete_recorded_before.business_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if limit <= 0 {
		return 0, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "limit must be positive")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	result, err := a.db.ExecContext(execCtx, `DELETE FROM business_events WHERE id IN (
		SELECT id FROM business_events WHERE recorded_at < $1 ORDER BY recorded_at, id LIMIT $2
	)`, cutoff, limit)
	if err != nil {
		return 0, businessEventStorageError(operation, err, "failed to delete business events")
	}
	deleted, err = result.RowsAffected()
	if err != nil {
		return 0, businessEventStorageError(operation, err, "failed to count deleted business events")
	}
	return deleted, nil
}

func businessEventStorageError(operation string, err error, message string) error {
	if isContextTimeoutOrCanceled(err) || isStatementTimeout(err) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	var pgErr *pgconn.PgError
	var connectErr *pgconn.ConnectError
	var netErr net.Error
	kind := storage.ErrorKindUnknown
	switch {
	case errors.As(err, &pgErr):
		switch {
		case pgErr.Code == "23505":
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "business event already exists")
		case strings.HasPrefix(pgErr.Code, "08"), pgErr.Code == "57P01", pgErr.Code == "57P02", pgErr.Code == "57P03":
			kind = storage.ErrorKindConnection
		}
	case errors.Is(err, driver.ErrBadConn), errors.Is(err, pgconn.ErrConnClosed),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.As(err, &connectErr), errors.As(err, &netErr):
		kind = storage.ErrorKindConnection
	}
	return storage.NewStorageError(operation, kind, err, message)
}

var _ ports.BusinessEventAppender = (*Adapter)(nil)
var _ ports.BusinessEventRetention = (*Adapter)(nil)
