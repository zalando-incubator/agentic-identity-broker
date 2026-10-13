package memory

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type BusinessEventRepository struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	registry     atomic.Pointer[businessEventValidation]
	events       map[model.BusinessEventKey]*model.BusinessEvent
	pending      map[model.BusinessEventKey]time.Time
	claimed      map[model.BusinessEventKey]bool
	retention    time.Duration
	now          func() time.Time
}

type businessEventValidation struct {
	validator ports.BusinessEventValidator
}

func NewBusinessEventRepository(transactions *TransactionManager, registry *ledger.Registry) *BusinessEventRepository {
	repository := &BusinessEventRepository{
		transactions: transactions,
		events:       make(map[model.BusinessEventKey]*model.BusinessEvent),
		pending:      make(map[model.BusinessEventKey]time.Time),
		claimed:      make(map[model.BusinessEventKey]bool),
		retention:    2160 * time.Hour,
		now:          time.Now,
	}
	repository.ConfigureBusinessEventValidation(registry)
	return repository
}

func (r *BusinessEventRepository) ConfigureBusinessEventValidation(validator ports.BusinessEventValidator) {
	r.registry.Store(&businessEventValidation{validator: validator})
}

func (r *BusinessEventRepository) Append(ctx context.Context, event *model.BusinessEvent, queueDelivery bool) error {
	const operation = "BusinessEvents.Append"
	if event == nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "business event is required")
	}
	txCtx, err := r.transactions.BeginTX(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = r.transactions.Rollback(txCtx) }()
	if err := r.appendInTransaction(txCtx, event, queueDelivery); err != nil {
		return err
	}
	return r.transactions.Commit(txCtx)
}

func (r *BusinessEventRepository) appendInTransaction(ctx context.Context, event *model.BusinessEvent, queueDelivery bool) error {
	const operation = "BusinessEvents.Append"
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return err
	}
	defer guard.release()
	if !event.RecordingPrepared() {
		event.PrepareRecording(r.now())
	}
	wire, err := r.registry.Load().validator.Validate(event)
	if err != nil {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "invalid business event")
	}
	key := model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
	r.mu.Lock()
	if _, exists := r.events[key]; exists {
		r.mu.Unlock()
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "business event already exists")
	}
	stored := copyBusinessEvent(event)
	slices.SortFunc(stored.PermissionSetIDs, func(a, b id.PermissionSetID) int { return bytes.Compare(a[:], b[:]) })
	stored.PermissionSetIDs = slices.Compact(stored.PermissionSetIDs)
	if _, trusted := wire["mcp_session_id"]; !trusted {
		stored.MCPSessionID = ""
	}
	if _, trusted := wire["agent_session_id"]; !trusted {
		stored.AgentSessionID = ""
	}
	stored.SetTrustedSessionCorrelations(stored.MCPSessionID, stored.AgentSessionID)
	journalEntry(ctx, r.events, key)
	r.events[key] = stored
	if queueDelivery {
		r.insertDelivery(ctx, key)
	}
	r.mu.Unlock()
	return nil
}

func (r *BusinessEventRepository) Get(ctx context.Context, key model.BusinessEventKey) (*model.BusinessEvent, error) {
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	key.RecordedAt = key.RecordedAt.UTC()
	r.mu.RLock()
	defer r.mu.RUnlock()
	event, found := r.events[key]
	if !found {
		return nil, storage.NewStorageError("BusinessEvents.Get", storage.ErrorKindNotFound, ports.ErrNotFound, "business event not found")
	}
	return copyBusinessEvent(event), nil
}

func (r *BusinessEventRepository) Query(ctx context.Context, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	if err := r.registry.Load().validator.ValidateQuery(query); err != nil {
		return nil, storage.NewStorageError("BusinessEvents.Query", storage.ErrorKindValidation, nil, "invalid business event query")
	}
	guard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	matches := make([]*model.BusinessEvent, 0)
	for _, event := range r.events {
		if event.Subject == nil {
			if !query.Subject.NoSubject {
				continue
			}
		} else if query.Subject.NoSubject || *event.Subject != query.Subject.Principal {
			continue
		}
		if event.OccurredAt.Before(query.Start) || !event.OccurredAt.Before(query.End) ||
			query.Type != "" && event.Type != query.Type || query.Outcome != "" && event.Outcome != query.Outcome {
			continue
		}
		if cursor := query.After; cursor != nil {
			if event.OccurredAt.Before(cursor.OccurredAt) || event.OccurredAt.Equal(cursor.OccurredAt) && bytes.Compare(event.ID[:], cursor.ID[:]) <= 0 {
				continue
			}
		}
		matches = append(matches, event)
	}
	slices.SortFunc(matches, func(a, b *model.BusinessEvent) int {
		if order := a.OccurredAt.Compare(b.OccurredAt); order != 0 {
			return order
		}
		return bytes.Compare(a.ID[:], b.ID[:])
	})
	limit := query.Limit
	if limit == 0 {
		limit = 200
	}
	if len(matches) > limit {
		matches = matches[:limit]
	}
	for i, event := range matches {
		matches[i] = copyBusinessEvent(event)
	}
	return matches, nil
}

func copyBusinessEvent(event *model.BusinessEvent) *model.BusinessEvent {
	copy := *event
	copy.Subject = copyPointer(event.Subject)
	copy.Actor.ID = copyPointer(event.Actor.ID)
	copy.Actor.OnBehalfOf = copyPointer(event.Actor.OnBehalfOf)
	copy.Client = copyPointer(event.Client)
	copy.PermissionSetIDs = slices.Clone(event.PermissionSetIDs)
	copy.Data = make(map[string]any, len(event.Data))
	for key, value := range event.Data {
		copy.Data[key] = copyJSONValue(value)
	}
	return &copy
}
