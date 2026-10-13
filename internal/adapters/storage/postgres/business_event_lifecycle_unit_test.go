package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func requireSafeLifecycleStorageError(t *testing.T, err error, kind storage.ErrorKind) {
	t.Helper()
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	require.Equal(t, kind, storageErr.Kind)
	require.NotContains(t, err.Error(), "business-event-sensitive-principal-canary")
	if storageErr.Cause != nil {
		require.NotContains(t, storageErr.Cause.Error(), "business-event-sensitive-principal-canary")
	}
}

func TestBusinessEventLifecycle_RejectsInvalidSelectorsBeforeDatabaseAccess(t *testing.T) {
	repo := NewBusinessEventLifecycleRepository(&Adapter{})
	for _, test := range []struct {
		name    string
		subject id.Principal
	}{
		{name: "empty"},
		{name: "whitespace", subject: id.NewPrincipal("  \t  ")},
		{name: "NUL", subject: id.NewPrincipal("business-event-sensitive-principal-canary\x00")},
	} {
		t.Run(test.name, func(t *testing.T) {
			deleted, err := repo.EraseSubject(context.Background(), test.subject)
			require.Zero(t, deleted)
			requireSafeLifecycleStorageError(t, err, storage.ErrorKindValidation)
		})
	}
	_, err := repo.EraseSubject(context.Background(), id.NewPrincipal("business-event-sensitive-principal-canary"))
	requireSafeLifecycleStorageError(t, err, storage.ErrorKindConnection)
}

func TestBusinessEventLifecycle_RejectsInvalidPolicyBeforeDatabaseAccess(t *testing.T) {
	repo := NewBusinessEventLifecycleRepository(&Adapter{})
	for _, duration := range []time.Duration{0, -time.Nanosecond, -time.Hour} {
		t.Run(duration.String(), func(t *testing.T) {
			requireSafeLifecycleStorageError(t, repo.SetRetentionPolicy(context.Background(), duration), storage.ErrorKindValidation)
		})
	}
	requireSafeLifecycleStorageError(t, repo.SetRetentionPolicy(context.Background(), time.Nanosecond), storage.ErrorKindConnection)
	requireSafeLifecycleStorageError(t, repo.ApplyRetention(context.Background()), storage.ErrorKindConnection)
}

func TestBusinessEventLifecycle_MapsStorageFailuresWithoutEventValues(t *testing.T) {
	const canary = "business-event-sensitive-principal-canary"
	for _, test := range []struct {
		name  string
		cause error
		kind  storage.ErrorKind
	}{
		{name: "deadline", cause: context.DeadlineExceeded, kind: storage.ErrorKindTimeout},
		{name: "cancellation", cause: context.Canceled, kind: storage.ErrorKindTimeout},
		{name: "permission", cause: &pgconn.PgError{Code: "42501", Detail: canary}, kind: storage.ErrorKindConnection},
		{name: "lock timeout", cause: &pgconn.PgError{Code: "55P03", Detail: canary}, kind: storage.ErrorKindTimeout},
		{name: "constraint", cause: &pgconn.PgError{Code: "23514", Detail: canary}, kind: storage.ErrorKindConnection},
		{name: "unique", cause: &pgconn.PgError{Code: "23505", Detail: canary}, kind: storage.ErrorKindConflict},
		{name: "generic SQL failure", cause: errors.New("row contains " + canary), kind: storage.ErrorKindConnection},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := businessEventStorageError("BusinessEventLifecycle.EraseSubject", test.cause)
			requireSafeLifecycleStorageError(t, err, test.kind)
		})
	}
}
