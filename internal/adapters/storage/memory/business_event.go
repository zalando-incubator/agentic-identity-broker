package memory

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Append stores an isolated occurrence and returns its storage time to the caller.
func (a *Adapter) Append(ctx context.Context, event *model.BusinessEvent) error {
	const operation = "Append"
	if err := ctx.Err(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event cannot be nil")
	}
	if err := event.Validate(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid business event")
	}

	if err := a.ledgerLock.Acquire(ctx, 1); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	defer a.ledgerLock.Release(1)
	for _, stored := range a.businessEvents {
		if stored.ID == event.ID {
			return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "business event already exists")
		}
	}
	stored := event.Copy()
	if stored.Attributes == nil {
		stored.Attributes = make(map[string]any)
	}
	if err := ctx.Err(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	stored.RecordedAt = time.Now().UTC()
	a.businessEvents = append(a.businessEvents, stored)
	event.RecordedAt = stored.RecordedAt
	return nil
}

func (a *Adapter) StorageTime(ctx context.Context) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, storage.NewStorageError("StorageTime", storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	return time.Now().UTC(), nil
}

// DeleteRecordedBefore removes at most limit occurrences, oldest storage time first.
func (a *Adapter) DeleteRecordedBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	const operation = "DeleteRecordedBefore"
	if err := ctx.Err(); err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	if limit <= 0 {
		return 0, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "limit must be positive")
	}
	if err := a.ledgerLock.Acquire(ctx, 1); err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	defer a.ledgerLock.Release(1)
	slices.SortFunc(a.businessEvents, func(left, right *model.BusinessEvent) int {
		if ordering := left.RecordedAt.Compare(right.RecordedAt); ordering != 0 {
			return ordering
		}
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	if err := ctx.Err(); err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	deleted := 0
	for deleted < len(a.businessEvents) && deleted < limit && a.businessEvents[deleted].RecordedAt.Before(cutoff) {
		deleted++
	}
	clear(a.businessEvents[:deleted])
	a.businessEvents = a.businessEvents[deleted:]
	if len(a.businessEvents) == 0 {
		a.businessEvents = nil
	}
	return int64(deleted), nil
}

var _ ports.BusinessEventAppender = (*Adapter)(nil)
var _ ports.BusinessEventRetention = (*Adapter)(nil)
