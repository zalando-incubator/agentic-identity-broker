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
func (a *Adapter) Append(ctx context.Context, event *model.DomainEvent) (err error) {
	const operation = "Append"
	ctx, finish := startStorageSpan(ctx, "storage.append.domain_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "domain event cannot be nil")
	}
	if err := event.Validate(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid domain event")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	if err := execCtx.Err(); err != nil {
		return domainEventStorageError(operation, err, "failed to append domain event")
	}
	var attributes any = "{}"
	if event.Attributes != nil {
		attributes, err = json.Marshal(event.Attributes)
		if err != nil {
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid domain event attributes")
		}
	}
	var recordedAt time.Time
	err = sqlx.GetContext(execCtx, a.db, &recordedAt, `INSERT INTO domain_events
		(id, event, occurred_at, principal, actor, trace_id, attributes)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING recorded_at`, event.ID, event.Event, event.OccurredAt.UTC(),
		event.Principal, event.Actor, event.TraceID, attributes)
	if err != nil {
		return domainEventStorageError(operation, err, "failed to append domain event")
	}
	event.RecordedAt = recordedAt
	return nil
}

func (a *Adapter) StorageTime(ctx context.Context) (now time.Time, err error) {
	const operation = "StorageTime"
	ctx, finish := startStorageSpan(ctx, "storage.current_time.domain_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return time.Time{}, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()
	if err := sqlx.GetContext(execCtx, a.db, &now, `SELECT statement_timestamp()`); err != nil {
		return time.Time{}, domainEventStorageError(operation, err, "failed to read domain event storage time")
	}
	return now, nil
}

// DeleteRecordedBefore uses one bounded statement outside any transaction carried by ctx.
func (a *Adapter) DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (deleted int64, err error) {
	const operation = "DeleteRecordedBefore"
	ctx, finish := startStorageSpan(ctx, "storage.delete_recorded_before.domain_event", operation)
	defer func() { finish(err) }()
	if a.db == nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	if limit <= 0 {
		return 0, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "limit must be positive")
	}
	execCtx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
	defer cancel()
	// Stored timestamps are whole microseconds. Round up to preserve the strict comparison.
	if floor := cutoff.Truncate(time.Microsecond); !floor.Equal(cutoff) {
		cutoff = floor.Add(time.Microsecond)
	}
	result, err := a.db.ExecContext(execCtx, `DELETE FROM domain_events WHERE id IN (
		SELECT id FROM domain_events WHERE recorded_at < $1 ORDER BY recorded_at, id LIMIT $2
	)`, cutoff, limit)
	if err != nil {
		return 0, domainEventStorageError(operation, err, "failed to delete domain events")
	}
	deleted, err = result.RowsAffected()
	if err != nil {
		return 0, domainEventStorageError(operation, err, "failed to count deleted domain events")
	}
	return deleted, nil
}

func domainEventStorageError(operation string, err error, message string) error {
	if isContextTimeoutOrCanceled(err) || isStatementTimeout(err) {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	var pgErr *pgconn.PgError
	var connectErr *pgconn.ConnectError
	var netErr net.Error
	kind := storage.ErrorKindUnknown
	switch {
	case errors.As(err, &connectErr):
		kind = storage.ErrorKindConnection
	case errors.As(err, &pgErr):
		switch {
		case pgErr.Code == "23505":
			return storage.NewStorageError(operation, storage.ErrorKindConflict, err, "domain event already exists")
		case strings.HasPrefix(pgErr.Code, "08"), pgErr.Code == "57P01", pgErr.Code == "57P02", pgErr.Code == "57P03":
			kind = storage.ErrorKindConnection
		}
	case errors.Is(err, driver.ErrBadConn), errors.Is(err, pgconn.ErrConnClosed),
		errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF),
		errors.As(err, &netErr):
		kind = storage.ErrorKindConnection
	}
	return storage.NewStorageError(operation, kind, err, message)
}

var _ ports.DomainEventAppender = (*Adapter)(nil)
var _ ports.DomainEventRetention = (*Adapter)(nil)
