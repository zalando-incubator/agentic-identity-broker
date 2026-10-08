package memory

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (r *BusinessEventRepository) beginLifecycle(ctx context.Context) (context.Context, error) {
	txCtx, err := r.transactions.BeginTX(ports.WithStorageTransactionHints(ctx, ports.StorageTransactionHints{Lifecycle: ports.StorageLifecycleExclusive}))
	if err != nil {
		return nil, err
	}
	scope, _ := memoryTransaction(txCtx)
	if !scope.owning {
		if err := r.transactions.Commit(txCtx); err != nil {
			return nil, err
		}
		return nil, storage.NewStorageError("BusinessEvents.Lifecycle", storage.ErrorKindConflict, nil, "lifecycle operation requires an owning transaction")
	}
	return txCtx, nil
}

// deleteBusinessEvent is called under the repository lock in an owning transaction.
func (r *BusinessEventRepository) deleteBusinessEvent(ctx context.Context, key model.BusinessEventKey) {
	journalEntry(ctx, r.events, key)
	delete(r.events, key)
	if _, pending := r.pending[key]; pending {
		journalEntry(ctx, r.pending, key)
		delete(r.pending, key)
	}
}

func (r *BusinessEventRepository) EraseSubject(ctx context.Context, subject id.Principal) (int64, error) {
	if err := ledger.ValidateErasureSubject(subject); err != nil {
		return 0, err
	}
	txCtx, err := r.beginLifecycle(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = r.transactions.Rollback(txCtx) }()

	r.mu.Lock()
	var removed int64
	for key, event := range r.events {
		if event.Subject != nil && *event.Subject == subject {
			r.deleteBusinessEvent(txCtx, key)
			removed++
		}
	}
	r.mu.Unlock()
	if err := r.transactions.Commit(txCtx); err != nil {
		return 0, err
	}
	return removed, nil
}

func (r *BusinessEventRepository) ApplyRetention(ctx context.Context) error {
	txCtx, err := r.beginLifecycle(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.transactions.Rollback(txCtx) }()

	r.mu.Lock()
	cutoff := r.now().UTC().Add(-r.retention)
	for key := range r.events {
		upperBound := key.RecordedAt.UTC().Truncate(6 * time.Hour).Add(6 * time.Hour)
		if !upperBound.After(cutoff) {
			r.deleteBusinessEvent(txCtx, key)
		}
	}
	r.mu.Unlock()
	return r.transactions.Commit(txCtx)
}

func (r *BusinessEventRepository) SetRetentionPolicy(ctx context.Context, retention time.Duration) error {
	normalized, err := ledger.NormalizeRetention(retention)
	if err != nil {
		return err
	}
	txCtx, err := r.beginLifecycle(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.transactions.Rollback(txCtx) }()

	r.mu.Lock()
	if r.retention != normalized {
		previous := r.retention
		scope, _ := memoryTransaction(txCtx)
		scope.owner.mu.Lock()
		scope.owner.undo = append(scope.owner.undo, func() { r.retention = previous })
		scope.owner.mu.Unlock()
		r.retention = normalized
	}
	r.mu.Unlock()
	return r.transactions.Commit(txCtx)
}
