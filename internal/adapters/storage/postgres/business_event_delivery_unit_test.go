package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventDelivery_RejectsCandidateLimitsBeforeDatabaseAccess(t *testing.T) {
	repo := NewBusinessEventRepository(nil, nil)
	for _, test := range []struct {
		name  string
		limit int
	}{
		{name: "negative", limit: -1},
		{name: "zero", limit: 0},
		{name: "above batch maximum", limit: 101},
	} {
		t.Run(test.name, func(t *testing.T) {
			keys, err := repo.ListDue(context.Background(), test.limit)
			require.Empty(t, keys)
			assertSafeBusinessEventStorageError(t, err, storage.ErrorKindValidation)
		})
	}
	for _, limit := range []int{1, 100} {
		keys, err := repo.ListDue(context.Background(), limit)
		require.Empty(t, keys)
		assertSafeBusinessEventStorageError(t, err, storage.ErrorKindConnection)
	}
}

func TestBusinessEventDelivery_MapsDatabaseFailuresWithoutEventValues(t *testing.T) {
	const canary = "business-event-sensitive-principal-canary"
	for _, test := range []struct {
		name  string
		cause error
		kind  storage.ErrorKind
	}{
		{name: "deadline", cause: context.DeadlineExceeded, kind: storage.ErrorKindTimeout},
		{name: "cancellation", cause: context.Canceled, kind: storage.ErrorKindTimeout},
		{name: "permission details", cause: &pgconn.PgError{Code: "42501", Detail: canary}, kind: storage.ErrorKindConnection},
		{name: "lock timeout details", cause: &pgconn.PgError{Code: "55P03", Detail: canary}, kind: storage.ErrorKindTimeout},
		{name: "unclassified details", cause: errors.New("row contains " + canary), kind: storage.ErrorKindConnection},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{
				queryErr: test.cause,
				beginErr: test.cause,
			}).adapter
			repo := NewBusinessEventRepository(adapter, nil)
			keys, err := repo.ListDue(context.Background(), 1)
			require.Empty(t, keys)
			assertSafeBusinessEventStorageError(t, err, test.kind)

			_, event := unitLedgerEvent(t)
			key := model.BusinessEventKey{RecordedAt: event.RecordedAt, ID: event.ID}
			invoked := false
			sent, err := repo.DispatchOne(context.Background(), key, func(context.Context, *model.BusinessEvent) error {
				invoked = true
				return nil
			})
			require.False(t, sent)
			require.False(t, invoked, "a failed claim cannot send a speculative payload")
			assertSafeBusinessEventStorageError(t, err, test.kind)
		})
	}
}
