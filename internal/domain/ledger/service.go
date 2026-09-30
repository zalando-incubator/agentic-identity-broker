package ledger

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type Service struct {
	events    ports.BusinessEventRepository
	lifecycle ports.BusinessEventLifecycleRepository
}

func NewService(events ports.BusinessEventRepository, lifecycle ports.BusinessEventLifecycleRepository) *Service {
	return &Service{events: events, lifecycle: lifecycle}
}

func (s *Service) Validate(*model.BusinessEvent) error {
	return nil
}

func (s *Service) Record(ctx context.Context, event *model.BusinessEvent) error {
	if err := s.Validate(event); err != nil {
		return err
	}
	return s.events.Append(ctx, event, false)
}

func (s *Service) Query(ctx context.Context, query model.BusinessEventQuery) ([]*model.BusinessEvent, error) {
	return s.events.Query(ctx, query)
}

func (s *Service) Get(ctx context.Context, key model.BusinessEventKey) (*model.BusinessEvent, error) {
	return s.events.Get(ctx, key)
}

func (s *Service) EraseSubject(ctx context.Context, subject id.Principal) (int64, error) {
	return s.lifecycle.EraseSubject(ctx, subject)
}

func (s *Service) ApplyRetention(ctx context.Context) error {
	return s.lifecycle.ApplyRetention(ctx)
}

func (s *Service) SetRetentionPolicy(ctx context.Context, retention time.Duration) error {
	return s.lifecycle.SetRetentionPolicy(ctx, retention)
}
