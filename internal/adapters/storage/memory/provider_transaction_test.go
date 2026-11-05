package memory

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestProviderDCRIdentityRollback(t *testing.T) {
	for _, operation := range []string{"create", "update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()
			tx := NewTransactionManager()
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository(tx)
			provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
			if operation != "create" {
				require.NoError(t, repo.Create(ctx, provider))
			}
			owner, err := tx.BeginTX(ctx)
			require.NoError(t, err)
			switch operation {
			case "create":
				require.NoError(t, repo.Create(owner, provider))
			case "update":
				changed := provider.Copy()
				changed.ClientID = id.ClientID("replacement-client")
				completedAt := changed.DiscoveryStatus.LastAttemptAt.Add(time.Minute)
				changed.DiscoveryStatus.LastAttemptAt = &completedAt
				changed.DiscoveryStatus.LastSuccessAt = &completedAt
				require.NoError(t, repo.Update(owner, changed, nil))
			case "delete":
				require.NoError(t, repo.Delete(owner, provider.ID))
			}
			require.NoError(t, tx.Rollback(owner))
			duplicate := provider.Copy()
			duplicate.ID = id.NewServiceID()
			if operation == "create" {
				require.NoError(t, repo.Create(ctx, duplicate), "rolled-back creation must release the DCR identity")
			} else {
				requireStorageKind(t, repo.Create(ctx, duplicate), storage.ErrorKindConflict)
				duplicate.ClientID = id.ClientID("replacement-client")
				require.NoError(t, repo.Create(ctx, duplicate), "rollback must release the replacement identity")
			}
		})
	}
}

func TestProviderDiscoveryFailureTransactionIsolation(t *testing.T) {
	ctx := context.Background()
	tx := NewTransactionManager()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository(tx)
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	owner, err := tx.BeginTX(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(owner) })
	failedAt := before.DiscoveryStatus.LastAttemptAt.Add(time.Minute)
	require.NoError(t, repo.RecordDiscoveryFailure(owner, provider.ID, before.Version, failedAt, "resource_metadata_invalid"))
	inside, err := repo.Get(owner, provider.ID)
	require.NoError(t, err)
	require.Equal(t, "resource_metadata_invalid", *inside.DiscoveryStatus.FailureReason)
	readCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	_, err = repo.Get(readCtx, provider.ID)
	require.Error(t, err, "uncommitted status must not be readable")
	require.NoError(t, tx.Rollback(owner))
	after, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestProviderDiscoveryFailureWaitsForTransaction(t *testing.T) {
	ctx := context.Background()
	tx := NewTransactionManager()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository(tx)
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, provider))
	owner, err := tx.BeginTX(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(owner) })
	writeCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	err = repo.RecordDiscoveryFailure(writeCtx, provider.ID, provider.Version, provider.DiscoveryStatus.LastAttemptAt.Add(time.Minute), "resource_metadata_invalid")
	require.Error(t, err, "standalone status writes must wait for the visibility gate")
	require.NoError(t, tx.Rollback(owner))
	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.Nil(t, stored.DiscoveryStatus.FailureReason)
}

func TestSessionCreationDoesNotDeadlockWithProviderUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tx := NewTransactionManager()
	sessions := NewInMemoryUserSessionRepository(tx)
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository(tx).WithUserSessionRepository(sessions)
	provider := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, provider))
	owner, err := tx.BeginTX(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(owner) })
	updated := provider.Copy()
	updated.DisplayName = "Updated provider"
	updateDone := make(chan error, 1)
	go func() { updateDone <- repo.Update(ctx, updated, nil) }()
	select {
	case err := <-updateDone:
		t.Fatalf("provider update passed the owning transaction: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	session := refreshTestSession(id.Principal("consenting-user"), provider.ID)
	createDone := make(chan error, 1)
	go func() { createDone <- sessions.Create(owner, session) }()
	select {
	case err := <-createDone:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("session creation deadlocked with provider update")
	}
	require.NoError(t, tx.Commit(owner))
	require.NoError(t, <-updateDone)
	stored, err := sessions.Get(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, stored.ID)
}
