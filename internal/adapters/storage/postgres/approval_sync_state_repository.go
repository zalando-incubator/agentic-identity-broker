package postgres

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ApprovalSyncStateRepository implements ApprovalSyncStateRepository using PostgreSQL.
type ApprovalSyncStateRepository struct {
	adapter *Adapter
}

// NewApprovalSyncStateRepository creates a new PostgreSQL approval sync state repository.
func NewApprovalSyncStateRepository(adapter *Adapter) *ApprovalSyncStateRepository {
	return &ApprovalSyncStateRepository{adapter: adapter}
}

var _ ports.ApprovalSyncStateRepository = (*ApprovalSyncStateRepository)(nil)

// GetVersion retrieves the current sync version from the approval_sync_state table.
func (r *ApprovalSyncStateRepository) GetVersion(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.get_version.approval_sync")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "GetApprovalSyncVersion"),
	)

	if r.adapter.db == nil {
		return 0, storage.NewStorageError(
			"GetApprovalSyncVersion",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var version int64
	err := r.adapter.storageExecutor(queryCtx).QueryRowContext(queryCtx, `SELECT version FROM approval_sync_state WHERE id = 1`).Scan(&version)

	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return 0, storage.NewStorageError(
				"GetApprovalSyncVersion",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return 0, storage.NewStorageError(
			"GetApprovalSyncVersion",
			storage.ErrorKindConnection,
			err,
			"failed to get sync version",
		)
	}

	return version, nil
}

// IncrementVersion atomically increments the sync version and issues NOTIFY for cross-instance wake-up.
// Both UPDATE and NOTIFY are wrapped in the same transaction per ADR 014/FR-010.
func (r *ApprovalSyncStateRepository) IncrementVersion(ctx context.Context) (int64, error) {
	ctx, span := otel.Tracer("storage").Start(ctx, "storage.increment_version.approval_sync")
	defer span.End()
	span.SetAttributes(
		semconv.DBSystemKey.String("postgresql"),
		attribute.String("db.operation", "IncrementApprovalSyncVersion"),
	)

	if r.adapter.db == nil {
		return 0, storage.NewStorageError(
			"IncrementApprovalSyncVersion",
			storage.ErrorKindConnection,
			nil,
			"database not initialized",
		)
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, err := r.adapter.beginSQLTransaction(execCtx, nil)
	if err != nil {
		return 0, storage.NewStorageError(
			"IncrementApprovalSyncVersion",
			storage.ErrorKindConnection,
			err,
			"failed to begin transaction",
		)
	}
	defer func() { _ = tx.Rollback() }()

	var version int64
	err = tx.QueryRowContext(execCtx,
		`UPDATE approval_sync_state SET version = version + 1 WHERE id = 1 RETURNING version`,
	).Scan(&version)

	if err != nil {
		if strings.Contains(err.Error(), "context deadline exceeded") {
			return 0, storage.NewStorageError(
				"IncrementApprovalSyncVersion",
				storage.ErrorKindTimeout,
				err,
				"operation exceeded timeout",
			)
		}
		return 0, storage.NewStorageError(
			"IncrementApprovalSyncVersion",
			storage.ErrorKindConnection,
			err,
			"failed to increment sync version",
		)
	}

	// Issue NOTIFY within the same transaction for atomic wake-up (ADR 014)
	_, err = tx.ExecContext(execCtx, `NOTIFY approval_sync`)
	if err != nil {
		return 0, storage.NewStorageError(
			"IncrementApprovalSyncVersion",
			storage.ErrorKindConnection,
			err,
			"failed to issue NOTIFY",
		)
	}

	if err := tx.Commit(); err != nil {
		return 0, storage.NewStorageError(
			"IncrementApprovalSyncVersion",
			storage.ErrorKindConnection,
			err,
			"failed to commit transaction",
		)
	}

	return version, nil
}
