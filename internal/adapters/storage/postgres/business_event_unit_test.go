package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func unitLedgerEvent(t *testing.T) (*ledger.Registry, *model.BusinessEvent) {
	t.Helper()
	registry, err := ledger.NewRegistry(eventschemas.Schemas)
	require.NoError(t, err)
	const canary = "business-event-sensitive-principal-canary"
	subject := id.NewPrincipal(canary)
	actorID := subject.String()
	now := time.Now().UTC().Truncate(time.Microsecond)
	event := &model.BusinessEvent{
		ID: id.NewBusinessEventID(), Type: model.BusinessEventTypePrefix + "token-issued",
		Source: model.BusinessEventSource, OccurredAt: now, RecordedAt: now,
		Subject: &subject, Actor: model.BusinessEventActor{Kind: "user", ID: &actorID},
		Outcome:     model.BusinessEventSuccess,
		ReasonUser:  "The broker completes a non-exchange token issuance.",
		ReasonAdmin: "The broker completes a non-exchange token issuance.", Data: map[string]any{},
	}
	_, err = registry.Validate(event)
	require.NoError(t, err, "unit fixture must satisfy the actual published schema")
	return registry, event
}

func assertSafeBusinessEventStorageError(t *testing.T, err error, kind storage.ErrorKind) {
	t.Helper()
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	require.Equal(t, kind, storageErr.Kind)
	require.NotContains(t, err.Error(), "business-event-sensitive-principal-canary")
	if storageErr.Cause != nil {
		require.NotContains(t, storageErr.Cause.Error(), "business-event-sensitive-principal-canary")
	}
}

func TestBusinessEventRepository_NilDatabaseMapsStorageErrors(t *testing.T) {
	for _, tt := range []struct {
		name    string
		adapter *Adapter
	}{{name: "nil adapter"}, {name: "nil database", adapter: &Adapter{}}} {
		t.Run(tt.name, func(t *testing.T) {
			registry, event := unitLedgerEvent(t)
			repo := NewBusinessEventRepository(tt.adapter, registry)
			query := model.BusinessEventQuery{
				Subject: model.BusinessEventSubject{Principal: *event.Subject},
				Start:   event.OccurredAt.Add(-time.Minute), End: event.OccurredAt.Add(time.Minute),
			}
			t.Run("append", func(t *testing.T) {
				assertSafeBusinessEventStorageError(t, repo.Append(context.Background(), event, true), storage.ErrorKindConnection)
			})
			t.Run("query", func(t *testing.T) {
				_, err := repo.Query(context.Background(), query)
				assertSafeBusinessEventStorageError(t, err, storage.ErrorKindConnection)
			})
			t.Run("get", func(t *testing.T) {
				_, err := repo.Get(context.Background(), model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
				assertSafeBusinessEventStorageError(t, err, storage.ErrorKindConnection)
			})
		})
	}
}

func TestBusinessEventRepository_DatabaseDeadlineMapsTimeoutWithoutEventValues(t *testing.T) {
	registry, event := unitLedgerEvent(t)
	adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
		beginErr: context.DeadlineExceeded,
		execErr:  context.DeadlineExceeded,
		queryErr: context.DeadlineExceeded,
	}).adapter
	repo := NewBusinessEventRepository(adapter, registry)
	query := model.BusinessEventQuery{
		Subject: model.BusinessEventSubject{Principal: *event.Subject},
		Start:   event.OccurredAt.Add(-time.Minute), End: event.OccurredAt.Add(time.Minute),
	}
	t.Run("append", func(t *testing.T) {
		assertSafeBusinessEventStorageError(t, repo.Append(context.Background(), event, false), storage.ErrorKindTimeout)
	})
	t.Run("query", func(t *testing.T) {
		_, err := repo.Query(context.Background(), query)
		assertSafeBusinessEventStorageError(t, err, storage.ErrorKindTimeout)
	})
	t.Run("get", func(t *testing.T) {
		_, err := repo.Get(context.Background(), model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID})
		assertSafeBusinessEventStorageError(t, err, storage.ErrorKindTimeout)
	})
}
