package retention

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type Service struct {
	repo   ports.DomainEventRetention
	cfg    ports.DomainEventsRetentionConfig
	logger *slog.Logger
}

func NewService(repo ports.DomainEventRetention, cfg ports.DomainEventsRetentionConfig, logger *slog.Logger) (*Service, error) {
	if repo == nil || logger == nil {
		return nil, errors.New("domain event retention repository and logger are required")
	}
	if cfg.Period <= 0 || cfg.Interval <= 0 || cfg.BatchSize <= 0 {
		return nil, errors.New("domain event retention period, interval, and batch size must be positive")
	}
	return &Service{repo: repo, cfg: cfg, logger: logger}, nil
}

func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()
	for {
		s.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) sweep(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	now, err := s.repo.StorageTime(ctx)
	if err != nil {
		s.logFailure(ctx, err)
		return
	}
	cutoff := now.Add(-s.cfg.Period)
	for ctx.Err() == nil {
		deleted, err := s.repo.DeleteRecordedBefore(ctx, cutoff, s.cfg.BatchSize)
		if err != nil {
			s.logFailure(ctx, err)
			return
		}
		if deleted < int64(s.cfg.BatchSize) {
			return
		}
	}
}

func (s *Service) logFailure(ctx context.Context, err error) {
	if ctx.Err() != nil {
		return
	}
	kind := storage.ErrorKindUnknown
	var storageErr *storage.StorageError
	if errors.Is(err, context.DeadlineExceeded) {
		kind = storage.ErrorKindTimeout
	} else if errors.As(err, &storageErr) {
		kind = storageErr.Kind
	}
	s.logger.ErrorContext(ctx, "domain event retention failed", "event", "domain_event.ledger.retention_failed", "error_kind", kind)
}
