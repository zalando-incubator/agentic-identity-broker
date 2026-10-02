package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (r *BusinessEventRepository) ListDue(ctx context.Context, limit int) ([]model.BusinessEventKey, error) {
	const operation = "BusinessEventDelivery.ListDue"
	if limit < 1 || limit > 100 {
		return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "candidate limit must be between 1 and 100")
	}
	if err := r.requireDB(operation); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	rows, err := r.adapter.storageExecutor(queryCtx).QueryContext(queryCtx,
		`SELECT recorded_at, event_id FROM public.business_event_delivery_pending
		WHERE next_attempt_at <= clock_timestamp()
		ORDER BY next_attempt_at, recorded_at, event_id LIMIT $1`, limit)
	if err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	defer func() { _ = rows.Close() }()
	keys := make([]model.BusinessEventKey, 0, limit)
	for rows.Next() {
		var key model.BusinessEventKey
		if err := rows.Scan(&key.RecordedAt, &key.ID); err != nil {
			return nil, businessEventStorageError(operation, err)
		}
		key.RecordedAt = key.RecordedAt.UTC()
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, businessEventStorageError(operation, err)
	}
	return keys, nil
}

func (r *BusinessEventRepository) DispatchOne(ctx context.Context, key model.BusinessEventKey, emit func(context.Context, *model.BusinessEvent) error) (bool, error) {
	const operation = "BusinessEventDelivery.DispatchOne"
	if err := r.requireDB(operation); err != nil {
		return false, err
	}

	// This unlocked lookup chooses the exact subject gate only. The event and
	// pending reference are rechecked after that gate is held; the payload is
	// never read from this optimistic snapshot.
	lookupCtx, stopLookup := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	var subject sql.NullString
	err := r.adapter.db.QueryRowContext(lookupCtx,
		`SELECT subject FROM public.business_events WHERE recorded_at=$1 AND id=$2`, key.RecordedAt, key.ID).Scan(&subject)
	stopLookup()
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	hints := ports.StorageTransactionHintsFromContext(execCtx)
	if subject.Valid {
		hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: id.NewPrincipal(subject.String)})
	}
	txCtx, err := r.adapter.BeginTX(ports.WithStorageTransactionHints(execCtx, hints))
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}
	defer func() { _ = r.adapter.Rollback(txCtx) }()
	tx := r.adapter.storageExecutor(txCtx)
	var envelope []byte
	var expectedSubject any
	if subject.Valid {
		expectedSubject = subject.String
	}
	err = tx.QueryRowContext(txCtx, `SELECT e.envelope
		FROM public.business_event_delivery_pending d
		JOIN public.business_events e ON e.recorded_at=d.recorded_at AND e.id=d.event_id
		WHERE d.recorded_at=$1 AND d.event_id=$2 AND d.next_attempt_at <= clock_timestamp()
			AND e.subject IS NOT DISTINCT FROM $3::text
		FOR UPDATE OF d SKIP LOCKED`, key.RecordedAt, key.ID, expectedSubject).Scan(&envelope)
	if errors.Is(err, sql.ErrNoRows) {
		if err := r.adapter.Commit(txCtx); err != nil {
			return false, businessEventStorageError(operation, err)
		}
		return false, nil
	}
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}
	event, err := r.decodeEvent(envelope)
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}

	exportErr := emit(txCtx, event)
	if err := txCtx.Err(); err != nil {
		return false, businessEventStorageError(operation, err)
	}
	var result sql.Result
	if exportErr != nil {
		result, err = tx.ExecContext(txCtx, `UPDATE public.business_event_delivery_pending
			SET next_attempt_at=clock_timestamp() + interval '30 seconds'
			WHERE recorded_at=$1 AND event_id=$2`, key.RecordedAt, key.ID)
	} else {
		result, err = tx.ExecContext(txCtx, `DELETE FROM public.business_event_delivery_pending
			WHERE recorded_at=$1 AND event_id=$2`, key.RecordedAt, key.ID)
	}
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, businessEventStorageError(operation, err)
	}
	if affected != 1 {
		return false, storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "claimed reference was not updated")
	}
	if err := txCtx.Err(); err != nil {
		return false, businessEventStorageError(operation, err)
	}
	if err := r.adapter.Commit(txCtx); err != nil {
		return false, businessEventStorageError(operation, err)
	}
	if exportErr != nil {
		return false, businessEventStorageError(operation, exportErr)
	}
	// Joined scopes stage acknowledgement; only an owning commit makes it durable.
	return true, nil
}
