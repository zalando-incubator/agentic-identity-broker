package memory

import (
	"context"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"golang.org/x/sync/semaphore"
)

const memoryGateCapacity int64 = 1<<63 - 1

type TransactionManager struct {
	lifecycle  *semaphore.Weighted
	visibility *semaphore.Weighted
}

type memoryTransactionKey struct{}
type memoryOperationKey struct{}

type memoryOperation struct {
	manager *TransactionManager
	write   bool
}

type memoryTransactionOwner struct {
	mu           sync.Mutex
	manager      *TransactionManager
	isolation    ports.StorageTransactionIsolation
	lifecycle    ports.StorageLifecycleGate
	rollbackOnly bool
	completed    bool
	undo         []func()
	afterCommit  []func()
}

type memoryTransactionScope struct {
	owner     *memoryTransactionOwner
	owning    bool
	completed bool
}

func NewTransactionManager() *TransactionManager {
	return &TransactionManager{lifecycle: semaphore.NewWeighted(memoryGateCapacity), visibility: semaphore.NewWeighted(memoryGateCapacity)}
}

func (m *TransactionManager) BeginTX(ctx context.Context) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindTimeout, err, "transaction context ended")
	}
	hints := ports.StorageTransactionHintsFromContext(ctx)
	if hints.Isolation > ports.StorageSerializable || hints.Lifecycle > ports.StorageLifecycleExclusive {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindValidation, nil, "invalid transaction hints")
	}
	if scope, ok := memoryTransaction(ctx); ok {
		owner := scope.owner
		owner.mu.Lock()
		defer owner.mu.Unlock()
		if owner.manager != m || owner.completed || scope.completed || owner.rollbackOnly {
			return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "transaction is not active")
		}
		if hints.Isolation > owner.isolation || hints.Lifecycle > owner.lifecycle {
			return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "joined scope cannot strengthen transaction protection")
		}
		return context.WithValue(ctx, memoryTransactionKey{}, &memoryTransactionScope{owner: owner}), nil
	}
	if operation, ok := ctx.Value(memoryOperationKey{}).(memoryOperation); ok && operation.manager == m {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindConflict, nil, "transaction must begin before repository access")
	}
	lifecycleWeight := int64(1)
	if hints.Lifecycle == ports.StorageLifecycleExclusive {
		lifecycleWeight = memoryGateCapacity
	}
	if err := m.lifecycle.Acquire(ctx, lifecycleWeight); err != nil {
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindTimeout, err, "failed to acquire lifecycle gate")
	}
	if err := m.visibility.Acquire(ctx, memoryGateCapacity); err != nil {
		m.lifecycle.Release(lifecycleWeight)
		return nil, storage.NewStorageError("StorageTransaction.BeginTX", storage.ErrorKindTimeout, err, "failed to acquire visibility gate")
	}
	owner := &memoryTransactionOwner{manager: m, isolation: hints.Isolation, lifecycle: hints.Lifecycle}
	ctx = context.WithValue(ctx, memoryTransactionKey{}, &memoryTransactionScope{owner: owner, owning: true})
	return ports.WithStorageTransactionEffects(ctx, owner), nil
}

func (m *TransactionManager) Commit(ctx context.Context) error {
	scope, ok := memoryTransaction(ctx)
	if !ok || scope.owner.manager != m {
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindValidation, nil, "transaction missing from context")
	}
	owner := scope.owner
	owner.mu.Lock()
	if scope.completed || owner.completed {
		owner.mu.Unlock()
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindConflict, nil, "transaction is already complete")
	}
	scope.completed = true
	if err := ctx.Err(); err != nil {
		owner.rollbackOnly = true
	}
	if !scope.owning {
		owner.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindTimeout, err, "transaction context ended")
		}
		return nil
	}
	owner.completed = true
	if owner.rollbackOnly {
		for i := len(owner.undo) - 1; i >= 0; i-- {
			owner.undo[i]()
		}
		owner.undo, owner.afterCommit = nil, nil
		owner.mu.Unlock()
		owner.release()
		if err := ctx.Err(); err != nil {
			return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindTimeout, err, "transaction context ended")
		}
		return storage.NewStorageError("StorageTransaction.Commit", storage.ErrorKindConflict, nil, "transaction is rollback-only")
	}
	effects := owner.afterCommit
	owner.undo, owner.afterCommit = nil, nil
	owner.mu.Unlock()
	owner.release()
	for _, effect := range effects {
		effect()
	}
	return nil
}

func (m *TransactionManager) Rollback(ctx context.Context) error {
	scope, ok := memoryTransaction(ctx)
	if !ok || scope.owner.manager != m {
		return storage.NewStorageError("StorageTransaction.Rollback", storage.ErrorKindValidation, nil, "transaction missing from context")
	}
	owner := scope.owner
	owner.mu.Lock()
	if scope.completed || owner.completed {
		owner.mu.Unlock()
		return nil
	}
	scope.completed = true
	owner.rollbackOnly = true
	if !scope.owning {
		owner.mu.Unlock()
		return nil
	}
	owner.completed = true
	for i := len(owner.undo) - 1; i >= 0; i-- {
		owner.undo[i]()
	}
	owner.undo, owner.afterCommit = nil, nil
	owner.mu.Unlock()
	owner.release()
	return nil
}

func (owner *memoryTransactionOwner) release() {
	owner.manager.visibility.Release(memoryGateCapacity)
	weight := int64(1)
	if owner.lifecycle == ports.StorageLifecycleExclusive {
		weight = memoryGateCapacity
	}
	owner.manager.lifecycle.Release(weight)
}

func (owner *memoryTransactionOwner) AfterCommit(effect func()) error {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.completed || owner.rollbackOnly || effect == nil {
		return storage.NewStorageError("StorageTransaction.AfterCommit", storage.ErrorKindConflict, nil, "transaction cannot accept effects")
	}
	owner.afterCommit = append(owner.afterCommit, effect)
	return nil
}

func memoryTransaction(ctx context.Context) (*memoryTransactionScope, bool) {
	scope, ok := ctx.Value(memoryTransactionKey{}).(*memoryTransactionScope)
	return scope, ok
}

type memoryOperationGuard struct {
	manager *TransactionManager
	weight  int64
}

func (guard memoryOperationGuard) release() {
	if guard.manager != nil {
		guard.manager.visibility.Release(guard.weight)
		guard.manager.lifecycle.Release(1)
	}
}

func (m *TransactionManager) enter(ctx context.Context, write bool) (context.Context, memoryOperationGuard, error) {
	guard, err := m.lock(ctx, write)
	if err != nil {
		return nil, memoryOperationGuard{}, err
	}
	if guard.manager != nil {
		ctx = context.WithValue(ctx, memoryOperationKey{}, memoryOperation{manager: m, write: write})
	}
	return ctx, guard, nil
}

func (m *TransactionManager) lock(ctx context.Context, write bool) (memoryOperationGuard, error) {
	if err := ctx.Err(); err != nil {
		return memoryOperationGuard{}, storage.NewStorageError("MemoryStorage", storage.ErrorKindTimeout, err, "operation context ended")
	}
	if scope, ok := memoryTransaction(ctx); ok {
		owner := scope.owner
		owner.mu.Lock()
		active := owner.manager == m && !owner.completed && !scope.completed && !owner.rollbackOnly
		owner.mu.Unlock()
		if !active {
			return memoryOperationGuard{}, storage.NewStorageError("MemoryStorage", storage.ErrorKindConflict, nil, "transaction is not active")
		}
		return memoryOperationGuard{}, nil
	}
	if operation, ok := ctx.Value(memoryOperationKey{}).(memoryOperation); ok && operation.manager == m {
		if write && !operation.write {
			return memoryOperationGuard{}, storage.NewStorageError("MemoryStorage", storage.ErrorKindConflict, nil, "read operation cannot acquire write access")
		}
		return memoryOperationGuard{}, nil
	}
	if err := m.lifecycle.Acquire(ctx, 1); err != nil {
		return memoryOperationGuard{}, storage.NewStorageError("MemoryStorage", storage.ErrorKindTimeout, err, "failed to acquire lifecycle gate")
	}
	weight := int64(1)
	if write {
		weight = memoryGateCapacity
	}
	if err := m.visibility.Acquire(ctx, weight); err != nil {
		m.lifecycle.Release(1)
		return memoryOperationGuard{}, storage.NewStorageError("MemoryStorage", storage.ErrorKindTimeout, err, "failed to acquire visibility gate")
	}
	return memoryOperationGuard{manager: m, weight: weight}, nil
}

func journalEntry[K comparable, V any](ctx context.Context, values map[K]V, key K) {
	scope, ok := memoryTransaction(ctx)
	if !ok {
		return
	}
	previous, existed := values[key]
	owner := scope.owner
	owner.mu.Lock()
	owner.undo = append(owner.undo, func() {
		if existed {
			values[key] = previous
		} else {
			delete(values, key)
		}
	})
	owner.mu.Unlock()
}
