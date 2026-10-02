package postgres

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type BusinessEventLifecycleRepository struct {
	adapter *Adapter
}

func NewBusinessEventLifecycleRepository(adapter *Adapter) *BusinessEventLifecycleRepository {
	return &BusinessEventLifecycleRepository{adapter: adapter}
}

func (r *BusinessEventLifecycleRepository) EraseSubject(ctx context.Context, subject id.Principal) (int64, error) {
	const operation = "BusinessEventLifecycle.EraseSubject"
	if err := ledger.ValidateErasureSubject(subject); err != nil {
		return 0, businessEventStorageError(operation, err)
	}
	if err := r.requireDB(operation); err != nil {
		return 0, err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	hints := ports.StorageTransactionHintsFromContext(execCtx)
	hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: subject, Exclusive: true})
	txCtx, err := r.adapter.BeginTX(ports.WithStorageTransactionHints(execCtx, hints))
	if err != nil {
		return 0, businessEventStorageError(operation, err)
	}
	defer func() { _ = r.adapter.Rollback(txCtx) }()
	var deleted int64
	if err := r.adapter.storageExecutor(txCtx).QueryRowContext(txCtx,
		`SELECT public.business_event_erase_subject($1)`, subject.String()).Scan(&deleted); err != nil {
		return 0, businessEventStorageError(operation, err)
	}
	if err := r.adapter.Commit(txCtx); err != nil {
		return 0, businessEventStorageError(operation, err)
	}
	return deleted, nil
}

func (r *BusinessEventLifecycleRepository) ApplyRetention(ctx context.Context) error {
	const operation = "BusinessEventLifecycle.ApplyRetention"
	if err := r.requireDB(operation); err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	_, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx, `SELECT public.business_event_maintain_partitions()`)
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	return nil
}

func (r *BusinessEventLifecycleRepository) SetRetentionPolicy(ctx context.Context, retention time.Duration) error {
	const operation = "BusinessEventLifecycle.SetRetentionPolicy"
	normalized, err := ledger.NormalizeRetention(retention)
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	if err := r.requireDB(operation); err != nil {
		return err
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	hints := ports.StorageTransactionHintsFromContext(execCtx)
	hints.Lifecycle = ports.StorageLifecycleExclusive
	txCtx, err := r.adapter.BeginTX(ports.WithStorageTransactionHints(execCtx, hints))
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	defer func() { _ = r.adapter.Rollback(txCtx) }()
	result, err := r.adapter.storageExecutor(txCtx).ExecContext(txCtx,
		`UPDATE public.business_event_policy SET retention_microseconds=$1 WHERE singleton`, normalized.Microseconds())
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return businessEventStorageError(operation, err)
	}
	if rows != 1 {
		return businessEventStorageError(operation, storage.NewStorageError(operation, storage.ErrorKindConnection, nil,
			"business event retention policy is missing or corrupt"))
	}
	if err := r.adapter.Commit(txCtx); err != nil {
		return businessEventStorageError(operation, err)
	}
	return nil
}

func (r *BusinessEventLifecycleRepository) requireDB(operation string) error {
	if r == nil || r.adapter == nil || r.adapter.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	return nil
}

func (a *Adapter) businessEventPartitionsReady(ctx context.Context) error {
	var available bool
	err := a.db.GetContext(ctx, &available, `WITH boundary AS (
		SELECT date_bin(interval '6 hours', clock_timestamp(), timestamptz '2000-01-01 00:00:00+00') AS lower_bound
	)
	SELECT count(*) = 2 FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
	JOIN pg_class p ON p.oid = i.inhparent JOIN pg_namespace n ON n.oid = c.relnamespace
	CROSS JOIN boundary b
	WHERE i.inhparent IN (to_regclass('public.business_events'), to_regclass('public.business_event_delivery_pending'))
		AND n.nspname = 'public'
		AND c.relname = p.relname || '_' || to_char(b.lower_bound AT TIME ZONE 'UTC', 'YYYYMMDD_HH24')
		AND pg_get_expr(c.relpartbound, c.oid) = format('FOR VALUES FROM (%L) TO (%L)',
			b.lower_bound::text, (b.lower_bound + interval '6 hours')::text)`)
	if err != nil {
		return businessEventStorageError("BusinessEvents.HealthCheck", err)
	}
	if !available {
		return storage.NewStorageError("BusinessEvents.HealthCheck", storage.ErrorKindConnection, nil, "current business event partition pair is unavailable")
	}
	return nil
}
