package memory

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func (r *BusinessEventRepository) insertDelivery(ctx context.Context, key model.BusinessEventKey) {
	journalEntry(ctx, r.pending, key)
	r.pending[key] = key.RecordedAt
}

func (r *BusinessEventRepository) ListDue(ctx context.Context, limit int) ([]model.BusinessEventKey, error) {
	if limit < 1 || limit > 100 {
		return nil, storage.NewStorageError("BusinessEventDelivery.ListDue", storage.ErrorKindValidation, nil, "candidate limit must be between 1 and 100")
	}
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()

	r.mu.RLock()
	defer r.mu.RUnlock()
	now := r.now().UTC()
	var keys []model.BusinessEventKey
	compare := func(a, b model.BusinessEventKey) int {
		if order := r.pending[a].Compare(r.pending[b]); order != 0 {
			return order
		}
		if order := a.RecordedAt.Compare(b.RecordedAt); order != 0 {
			return order
		}
		return bytes.Compare(a.ID[:], b.ID[:])
	}
	for key, next := range r.pending {
		if next.After(now) || r.claimed[key] || r.events[key] == nil ||
			len(keys) == limit && compare(key, keys[len(keys)-1]) >= 0 {
			continue
		}
		index, _ := slices.BinarySearchFunc(keys, key, compare)
		if len(keys) < limit {
			if keys == nil {
				keys = make([]model.BusinessEventKey, 0, min(limit, len(r.pending)))
			}
			keys = append(keys, key)
		}
		copy(keys[index+1:], keys[index:len(keys)-1])
		keys[index] = key
	}
	return keys, nil
}

func (r *BusinessEventRepository) DispatchOne(ctx context.Context, key model.BusinessEventKey, emit func(context.Context, *model.BusinessEvent) error) (bool, error) {
	const operation = "BusinessEventDelivery.DispatchOne"
	// An ambient transaction owns exclusive visibility and can contain uncommitted
	// events. Never call a collector while that transaction is still open.
	if _, ok := memoryTransaction(ctx); ok {
		return false, storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "dispatch requires a committed event outside a transaction")
	}
	if operationCtx, ok := ctx.Value(memoryOperationKey{}).(memoryOperation); ok && operationCtx.manager == r.transactions {
		return false, storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "dispatch cannot run inside a storage operation")
	}
	if err := ctx.Err(); err != nil {
		return false, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "dispatch context ended")
	}
	key.RecordedAt = key.RecordedAt.UTC()
	r.mu.RLock()
	claimed := r.claimed[key]
	r.mu.RUnlock()
	if claimed {
		return false, nil
	}
	if err := r.transactions.lifecycle.Acquire(ctx, 1); err != nil {
		return false, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "failed to acquire lifecycle gate")
	}
	defer r.transactions.lifecycle.Release(1)

	// Acquire visibility directly: reentering transactions.lock would request
	// another lifecycle share and deadlock behind a queued exclusive erasure.
	if err := r.transactions.visibility.Acquire(ctx, memoryGateCapacity); err != nil {
		return false, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "failed to acquire visibility gate")
	}
	if err := ctx.Err(); err != nil {
		r.transactions.visibility.Release(memoryGateCapacity)
		return false, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "dispatch context ended")
	}
	r.mu.Lock()
	next, pending := r.pending[key]
	if !pending || next.After(r.now().UTC()) || r.claimed[key] || r.events[key] == nil {
		r.mu.Unlock()
		r.transactions.visibility.Release(memoryGateCapacity)
		return false, nil
	}
	retained := copyBusinessEvent(r.events[key])
	r.claimed[key] = true
	r.mu.Unlock()
	r.transactions.visibility.Release(memoryGateCapacity)
	defer func() {
		r.mu.Lock()
		delete(r.claimed, key)
		r.mu.Unlock()
	}()

	// The lifecycle share protects this copy against erasure and retention for
	// the entire synchronous callback, without blocking unrelated transactions.
	exportErr := ctx.Err()
	if exportErr == nil {
		exportErr = emit(ctx, retained)
	}
	ackCtx := ctx
	if exportErr == nil {
		exportErr = r.transactions.visibility.Acquire(ctx, memoryGateCapacity)
	}
	if exportErr != nil {
		var cancel context.CancelFunc
		ackCtx, cancel = context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		if r.transactions.visibility.Acquire(ackCtx, memoryGateCapacity) != nil {
			return false, exportErr
		}
	}
	defer r.transactions.visibility.Release(memoryGateCapacity)
	if err := ackCtx.Err(); err != nil {
		if exportErr != nil {
			return false, exportErr
		}
		return false, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "acknowledgement context ended")
	}
	r.mu.Lock()
	if exportErr == nil {
		exportErr = ctx.Err()
	}
	if exportErr == nil {
		delete(r.pending, key)
	} else {
		r.pending[key] = r.now().UTC().Add(30 * time.Second)
	}
	r.mu.Unlock()
	if exportErr != nil {
		return false, exportErr
	}
	return true, nil
}
