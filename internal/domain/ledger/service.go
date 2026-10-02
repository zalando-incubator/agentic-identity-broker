package ledger

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type Service struct {
	registry     *Registry
	events       ports.BusinessEventRepository
	lifecycle    ports.BusinessEventLifecycleRepository
	transactions ports.StorageTransactionManager
	copyEnabled  bool
}

// recordingSpans are carried by the outer ledger transaction so joined Record
// calls remain timed through the physical owner commit (or rollback).
type recordingSpans struct {
	first  trace.Span
	others []trace.Span
}

type recordingSpansKey struct{}

func NewService(registry *Registry, events ports.BusinessEventRepository, lifecycle ports.BusinessEventLifecycleRepository, transactions ports.StorageTransactionManager, copyEnabled bool) *Service {
	return &Service{registry: registry, events: events, lifecycle: lifecycle, transactions: transactions, copyEnabled: copyEnabled}
}

func (s *Service) Validate(event *model.BusinessEvent) error {
	_, err := s.registry.Validate(event)
	return err
}

func (s *Service) NewEvent(ctx context.Context, eventType string, facts model.BusinessEvent) (*model.BusinessEvent, error) {
	registered, exists := s.registry.events[eventType]
	if !exists {
		return nil, errInvalidEvent
	}
	facts = ContextFacts(ctx, facts)
	facts.ID = id.NewBusinessEventID()
	facts.Type, facts.Source = eventType, model.BusinessEventSource
	facts.Outcome = registered.outcome
	facts.ReasonUser, facts.ReasonAdmin = registered.reasonUser, registered.reasonAdmin
	facts.RecordedAt = time.Time{}
	return &facts, nil
}

func (s *Service) Record(ctx context.Context, event *model.BusinessEvent) error {
	ctx, span := otel.Tracer("ledger").Start(ctx, "ledger.record")
	if owner, ok := ctx.Value(recordingSpansKey{}).(*recordingSpans); ok {
		if owner.first == nil {
			owner.first = span
		} else {
			owner.others = append(owner.others, span)
		}
	} else {
		defer span.End()
	}
	if event == nil {
		return errInvalidEvent
	}
	candidate := *event
	if candidate.RecordedAt.IsZero() {
		candidate.RecordedAt = candidate.OccurredAt
	}
	if err := s.Validate(&candidate); err != nil {
		return err
	}
	hints := ports.StorageTransactionHintsFromContext(ctx)
	if event.Subject != nil {
		hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: *event.Subject})
	}
	return s.WithTransaction(ctx, hints, func(txCtx context.Context) error {
		return s.events.Append(txCtx, event, s.copyEnabled)
	})
}

func (s *Service) WithTransaction(ctx context.Context, hints ports.StorageTransactionHints, work func(context.Context) error) error {
	if _, ok := ctx.Value(recordingSpansKey{}).(*recordingSpans); !ok {
		owner := &recordingSpans{}
		ctx = context.WithValue(ctx, recordingSpansKey{}, owner)
		defer func() {
			if owner.first != nil {
				owner.first.End()
			}
			for _, span := range owner.others {
				span.End()
			}
		}()
	}
	txCtx, err := s.transactions.BeginTX(ports.WithStorageTransactionHints(ctx, hints))
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = s.transactions.Rollback(txCtx)
		}
	}()
	if err := work(txCtx); err != nil {
		return err
	}
	if err := s.transactions.Commit(txCtx); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Service) Query(ctx context.Context, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	if err := s.registry.ValidateQuery(query); err != nil {
		return nil, err
	}
	if query.Type != "" && query.Outcome != "" && query.Outcome != s.registry.events[query.Type].outcome {
		return nil, errInvalidQuery
	}
	return s.events.Query(ctx, query)
}

func (s *Service) Get(ctx context.Context, key model.BusinessEventKey) (*model.BusinessEvent, error) {
	return s.events.Get(ctx, key)
}
