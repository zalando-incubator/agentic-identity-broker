package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func serviceAccountFixture(serviceID id.ServiceID, canonicalID string) *model.ServiceAccount {
	expiry := time.Now().UTC().Add(time.Hour)
	return &model.ServiceAccount{
		ID: id.NewServiceAccountID(), ServiceID: serviceID, CanonicalID: canonicalID, Kind: model.ServiceAccountKindJWTBearer,
		Scopes: []string{"read"}, Credential: model.NewEncryptedSecret([]byte("ciphertext")),
		Summary:             model.CredentialSummary{Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken, Kid: new("2026-11"), Fingerprint: "SHA256:x"},
		CredentialExpiresAt: &expiry, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
}

func serviceAccountRepositoryWithService(t *testing.T) (*ServiceAccountRepository, id.ServiceID) {
	t.Helper()
	providers := NewInMemoryThirdpartyOAuth2ProviderRepository()
	repo := NewServiceAccountRepository().WithProviderRepository(providers)
	providers.WithServiceAccountRepository(repo)
	service := testProvider(id.NewServiceID())
	require.NoError(t, providers.Create(context.Background(), service))
	return repo, service.ID
}

func TestServiceAccountRepository_DeepCopies(t *testing.T) {
	ctx := context.Background()
	repo, serviceID := serviceAccountRepositoryWithService(t)
	account := serviceAccountFixture(serviceID, "platform-triage-bot")
	require.NoError(t, repo.Create(ctx, account))

	account.Scopes[0] = "caller mutation"
	*account.Summary.Kid = "caller mutation"
	*account.CredentialExpiresAt = time.Time{}

	stored, err := repo.Get(ctx, serviceID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"read"}, stored.Scopes)
	assert.Equal(t, "2026-11", *stored.Summary.Kid)
	assert.False(t, stored.CredentialExpiresAt.IsZero())

	stored.Scopes[0] = "reader mutation"
	*stored.Summary.Kid = "reader mutation"
	listed, err := repo.ListByService(ctx, serviceID)
	require.NoError(t, err)
	assert.Equal(t, []string{"read"}, listed[0].Scopes)
	assert.Equal(t, "2026-11", *listed[0].Summary.Kid)
}

func TestServiceAccountRepository_WithoutProviderRepositoryRefusesCreate(t *testing.T) {
	err := NewServiceAccountRepository().Create(context.Background(), serviceAccountFixture(id.NewServiceID(), "orphan"))
	require.Error(t, err)
}

func TestServiceAccountRepository_ConcurrentCreatesClaimANameOnce(t *testing.T) {
	ctx := context.Background()
	repo, serviceID := serviceAccountRepositoryWithService(t)
	var created atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if repo.Create(ctx, serviceAccountFixture(serviceID, "platform-triage-bot")) == nil {
				created.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), created.Load())
	count, err := repo.CountByService(ctx, serviceID)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestServiceAccountRepository_ConcurrentConditionalUpdatesApplyOnce(t *testing.T) {
	ctx := context.Background()
	repo, serviceID := serviceAccountRepositoryWithService(t)
	account := serviceAccountFixture(serviceID, "platform-triage-bot")
	require.NoError(t, repo.Create(ctx, account))
	var updated atomic.Int32
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			candidate := *account
			candidate.Scopes = []string{"scope-" + string(rune('a'+i))}
			if repo.Update(ctx, &candidate, new(int64(1))) == nil {
				updated.Add(1)
			}
		})
	}
	wg.Wait()
	assert.Equal(t, int32(1), updated.Load())
	stored, err := repo.Get(ctx, serviceID, account.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), stored.Version)
}

func TestServiceAccountRepository_CreateRacingServiceDeletion(t *testing.T) {
	ctx := context.Background()
	for range 20 {
		providers := NewInMemoryThirdpartyOAuth2ProviderRepository()
		repo := NewServiceAccountRepository().WithProviderRepository(providers)
		providers.WithServiceAccountRepository(repo)
		service := testProvider(id.NewServiceID())
		require.NoError(t, providers.Create(ctx, service))
		var wg sync.WaitGroup
		var createErr, deleteErr error
		wg.Go(func() { createErr = repo.Create(ctx, serviceAccountFixture(service.ID, "platform-triage-bot")) })
		wg.Go(func() { deleteErr = providers.Delete(ctx, service.ID) })
		wg.Wait()
		assert.True(t, (createErr == nil) != (deleteErr == nil), "exactly one of create and service delete succeeds: %v / %v", createErr, deleteErr)
	}
}
