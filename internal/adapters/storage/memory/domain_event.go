package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type storedDomainEvent struct {
	ID         id.DomainEventID
	Event      string
	OccurredAt time.Time
	RecordedAt time.Time
	Principal  *string
	Actor      *string
	TraceID    string
	attributes []byte
}

// Append stores an isolated occurrence and returns its storage time to the caller.
func (a *Adapter) Append(ctx context.Context, event *model.DomainEvent) error {
	const operation = "Append"
	if err := ctx.Err(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "domain event cannot be nil")
	}
	if err := event.Validate(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid domain event")
	}
	var attributes []byte
	if event.Attributes == nil {
		attributes = []byte("{}")
	} else {
		var err error
		attributes, err = json.Marshal(event.Attributes)
		if err != nil {
			return storage.NewStorageError(operation, storage.ErrorKindValidation, err, "invalid domain event attributes")
		}
	}

	if err := a.ledgerLock.Acquire(ctx, 1); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	defer a.ledgerLock.Release(1)
	for _, stored := range a.domainEvents {
		if stored.ID == event.ID {
			return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "domain event already exists")
		}
	}
	stored := &storedDomainEvent{
		ID: event.ID, Event: event.Event, OccurredAt: event.OccurredAt,
		TraceID: event.TraceID, attributes: attributes,
	}
	if event.Principal != nil {
		principal := *event.Principal
		stored.Principal = &principal
	}
	if event.Actor != nil {
		actor := *event.Actor
		stored.Actor = &actor
	}
	if err := ctx.Err(); err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	stored.RecordedAt = time.Now().UTC()
	a.domainEvents = append(a.domainEvents, stored)
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
	slices.SortFunc(a.domainEvents, func(left, right *storedDomainEvent) int {
		if ordering := left.RecordedAt.Compare(right.RecordedAt); ordering != 0 {
			return ordering
		}
		return bytes.Compare(left.ID[:], right.ID[:])
	})
	if err := ctx.Err(); err != nil {
		return 0, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	deleted := 0
	for deleted < len(a.domainEvents) && deleted < limit && a.domainEvents[deleted].RecordedAt.Before(cutoff) {
		deleted++
	}
	clear(a.domainEvents[:deleted])
	a.domainEvents = a.domainEvents[deleted:]
	if len(a.domainEvents) == 0 {
		a.domainEvents = nil
	}
	return int64(deleted), nil
}

var _ ports.DomainEventAppender = (*Adapter)(nil)
var _ ports.DomainEventRetention = (*Adapter)(nil)
