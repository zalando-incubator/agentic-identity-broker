package model

import (
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type DomainEvent struct {
	ID         id.DomainEventID
	Event      string
	OccurredAt time.Time
	RecordedAt time.Time
	Principal  *string
	Actor      *string
	TraceID    string
	Attributes map[string]any
}

func (e *DomainEvent) Validate() error {
	if e.ID.IsZero() {
		return errors.New("domain event ID is required")
	}
	if e.Event == "" {
		return errors.New("domain event identifier is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("domain event occurrence time is required")
	}
	return nil
}
