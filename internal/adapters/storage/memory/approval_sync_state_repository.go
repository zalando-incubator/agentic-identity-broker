package memory

import (
	"context"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// ApprovalSyncStateRepository implements ApprovalSyncStateRepository in-memory.
type ApprovalSyncStateRepository struct {
	mu           sync.Mutex
	transactions *TransactionManager
	version      int64
}

// NewApprovalSyncStateRepository creates a new in-memory ApprovalSyncStateRepository.
func NewApprovalSyncStateRepository(transactions *TransactionManager) *ApprovalSyncStateRepository {
	return &ApprovalSyncStateRepository{transactions: transactions}
}

var _ ports.ApprovalSyncStateRepository = (*ApprovalSyncStateRepository)(nil)

func (r *ApprovalSyncStateRepository) GetVersion(ctx context.Context) (int64, error) {
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return 0, err
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.version, nil
}

func (r *ApprovalSyncStateRepository) IncrementVersion(ctx context.Context) (int64, error) {
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return 0, err
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()
	if scope, ok := memoryTransaction(ctx); ok {
		previous := r.version
		scope.owner.mu.Lock()
		scope.owner.undo = append(scope.owner.undo, func() { r.version = previous })
		scope.owner.mu.Unlock()
	}
	r.version++
	return r.version, nil
}
