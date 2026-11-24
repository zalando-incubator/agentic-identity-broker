package model

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

func TestDomainEventValidate(t *testing.T) {
	valid := DomainEvent{ID: id.NewDomainEventID(), Event: "consent.grant.revoked", OccurredAt: time.Now()}
	require.NoError(t, valid.Validate())
	for _, tc := range []struct {
		name       string
		invalidate func(*DomainEvent)
	}{
		{"zero ID", func(e *DomainEvent) { e.ID = id.DomainEventID{} }},
		{"empty event", func(e *DomainEvent) { e.Event = "" }},
		{"zero occurrence time", func(e *DomainEvent) { e.OccurredAt = time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := valid
			tc.invalidate(&event)
			require.Error(t, event.Validate())
		})
	}
}
