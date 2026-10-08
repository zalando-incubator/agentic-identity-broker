package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
)

type BusinessEventRepository struct {
	adapter  *Adapter
	registry atomic.Pointer[businessEventValidation]
}

type businessEventValidation struct {
	validator ports.BusinessEventValidator
}

func NewBusinessEventRepository(adapter *Adapter, registry *ledger.Registry) *BusinessEventRepository {
	repository := &BusinessEventRepository{adapter: adapter}
	repository.ConfigureBusinessEventValidation(registry)
	return repository
}

func (r *BusinessEventRepository) ConfigureBusinessEventValidation(validator ports.BusinessEventValidator) {
	r.registry.Store(&businessEventValidation{validator: validator})
}

func (r *BusinessEventRepository) Append(ctx context.Context, event *model.BusinessEvent, queueDelivery bool) error {
	const operation = "BusinessEvents.Append"
	if err := r.requireDB(operation); err != nil {
		return err
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event is required")
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	hints := ports.StorageTransactionHintsFromContext(execCtx)
	if event.Subject != nil {
		hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: *event.Subject})
	}
	txCtx, err := r.adapter.BeginTX(ports.WithStorageTransactionHints(execCtx, hints))
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	defer func() { _ = r.adapter.Rollback(txCtx) }()
	scope, _ := storageTransaction(txCtx)
	if !event.RecordingPrepared() {
		var recordedAt time.Time
		if err := scope.GetContext(txCtx, &recordedAt, `SELECT clock_timestamp()`); err != nil {
			return businessEventStorageError(operation, err)
		}
		event.PrepareRecording(recordedAt)
	}
	wire, err := r.registry.Load().validator.Validate(event)
	if err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "invalid business event")
	}
	envelope, err := json.Marshal(wire)
	if err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event cannot be serialized")
	}
	const appendEvent = `INSERT INTO public.business_events
		(recorded_at,id,occurred_at,type,outcome,subject,envelope)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb)`
	const appendQueuedEvent = `WITH retained_event AS (` + appendEvent + `
		RETURNING recorded_at,id)
		INSERT INTO public.business_event_delivery_pending(recorded_at,event_id,next_attempt_at)
		SELECT recorded_at,id,recorded_at FROM retained_event`
	statement := appendEvent
	if queueDelivery {
		statement = appendQueuedEvent
	}
	_, err = scope.ExecContext(txCtx, statement, event.RecordedAt, event.ID, event.OccurredAt,
		event.Type, event.Outcome, event.Subject, envelope)
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	if err := r.adapter.Commit(txCtx); err != nil {
		return businessEventStorageError(operation, err)
	}
	return nil
}

func (r *BusinessEventRepository) Query(ctx context.Context, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	const operation = "BusinessEvents.Query"
	if err := r.requireDB(operation); err != nil {
		return nil, err
	}
	if err := r.registry.Load().validator.ValidateQuery(query); err != nil {
		return nil, storage.NewStorageError("BusinessEvents.Query", storage.ErrorKindValidation, nil, "invalid business event query")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var statement strings.Builder
	statement.WriteString(`SELECT envelope FROM public.business_events WHERE occurred_at >= $1 AND occurred_at < $2`)
	args := make([]any, 0, 8)
	args = append(args, ceilBusinessEventMicrosecond(query.Start), ceilBusinessEventMicrosecond(query.End))
	if query.Subject.NoSubject {
		statement.WriteString(` AND subject IS NULL`)
	} else {
		args = append(args, query.Subject.Principal.String())
		statement.WriteString(` AND subject = $` + strconv.Itoa(len(args)))
	}
	if query.Type != "" {
		args = append(args, query.Type)
		statement.WriteString(` AND type = $` + strconv.Itoa(len(args)))
	}
	if query.Outcome != "" {
		args = append(args, query.Outcome)
		statement.WriteString(` AND outcome = $` + strconv.Itoa(len(args)))
	}
	if cursor := query.After; cursor != nil {
		aligned := cursor.OccurredAt.Truncate(time.Microsecond)
		args = append(args, aligned)
		if aligned.Equal(cursor.OccurredAt) {
			position := strconv.Itoa(len(args))
			args = append(args, cursor.ID)
			statement.WriteString(` AND (occurred_at,id) > ($` + position + `,$` + strconv.Itoa(len(args)) + `)`)
		} else {
			statement.WriteString(` AND occurred_at > $` + strconv.Itoa(len(args)))
		}
	}
	limit := query.Limit
	if limit == 0 {
		limit = 200
	}
	args = append(args, limit)
	statement.WriteString(` ORDER BY occurred_at ASC,id ASC LIMIT $` + strconv.Itoa(len(args)))
	rows, err := r.adapter.storageExecutor(queryCtx).QueryContext(queryCtx, statement.String(), args...)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	defer func() { _ = rows.Close() }()
	events := make([]*model.BusinessEvent, 0)
	for rows.Next() {
		var envelope []byte
		if err := rows.Scan(&envelope); err != nil {
			return nil, businessEventStorageError(operation, err)
		}
		event, err := r.decodeEvent(envelope)
		if err != nil {
			return nil, businessEventStorageError(operation, err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	return events, nil
}

func (r *BusinessEventRepository) Get(ctx context.Context, key model.BusinessEventKey) (*model.BusinessEvent, error) {
	const operation = "BusinessEvents.Get"
	if err := r.requireDB(operation); err != nil {
		return nil, err
	}
	if key.ID.IsZero() || key.RecordedAt.IsZero() || key.RecordedAt.Nanosecond()%1000 != 0 {
		return nil, storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "business event not found")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var envelope []byte
	err := r.adapter.storageExecutor(queryCtx).QueryRowContext(queryCtx,
		`SELECT envelope FROM public.business_events WHERE recorded_at=$1 AND id=$2`, key.RecordedAt, key.ID).Scan(&envelope)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	event, err := r.decodeEvent(envelope)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	return event, nil
}

func (r *BusinessEventRepository) requireDB(operation string) error {
	if r.adapter == nil || r.adapter.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	return nil
}

func (r *BusinessEventRepository) decodeEvent(envelope []byte) (*model.BusinessEvent, error) {
	var event model.BusinessEvent
	if err := json.Unmarshal(envelope, &event); err != nil {
		return nil, storage.NewStorageError("BusinessEvents.Decode", storage.ErrorKindValidation, nil, "invalid retained business event")
	}
	event.SetTrustedSessionCorrelations(event.MCPSessionID, event.AgentSessionID)
	if _, err := r.registry.Load().validator.Validate(&event); err != nil {
		return nil, storage.NewStorageError("BusinessEvents.Decode", storage.ErrorKindValidation, nil, "invalid retained business event")
	}
	return &event, nil
}

func ceilBusinessEventMicrosecond(instant time.Time) time.Time {
	aligned := instant.Truncate(time.Microsecond)
	if !aligned.Equal(instant) {
		return aligned.Add(time.Microsecond)
	}
	return aligned
}

func businessEventStorageError(operation string, err error) error {
	kind := storage.ErrorKindConnection
	var cause error
	var existing *storage.StorageError
	var pgErr *pgconn.PgError
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind, cause = storage.ErrorKindTimeout, context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		kind, cause = storage.ErrorKindTimeout, context.Canceled
	case errors.Is(err, sql.ErrNoRows):
		kind, cause = storage.ErrorKindNotFound, ports.ErrNotFound
	case errors.As(err, &existing):
		kind = existing.Kind
	case errors.As(err, &pgErr):
		cause = &pgconn.PgError{Code: pgErr.Code}
		switch {
		case pgErr.Code == "23505":
			kind = storage.ErrorKindConflict
		case pgErr.Code == "23514":
			kind = storage.ErrorKindConnection
		case pgErr.Code == "57014" || pgErr.Code == "55P03":
			kind = storage.ErrorKindTimeout
		case strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23"):
			kind = storage.ErrorKindValidation
		}
	}
	return storage.NewStorageError(operation, kind, cause, "business event storage operation failed")
}
