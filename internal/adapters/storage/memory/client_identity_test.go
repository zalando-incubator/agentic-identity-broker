package memory

import (
	"context"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/storagecontracts"
	"math"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIdentityRepositoryLegacyAndExhaustion(t *testing.T) {
	ctx := context.Background()
	for _, next := range []id.ClientID{"client-id", "different"} {
		repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
		provider := testProvider(id.NewServiceID())
		require.NoError(t, repo.Create(ctx, provider))
		repo.providers[provider.ID].entity.ClientIdentity = nil
		provider.ClientID = next
		changed, err := repo.Update(ctx, provider, &provider.Version)
		require.NoError(t, err)
		assert.Equal(t, next != "client-id", changed)
		assert.Equal(t, new(next), provider.ClientIdentity)
		expected := int64(0)
		if changed {
			expected = 1
		}
		assert.Equal(t, expected, provider.ClientIdentityRevision)
		repo.providers[provider.ID].entity.ClientIdentityRevision = math.MaxInt64
		before, err := repo.Get(ctx, provider.ID)
		require.NoError(t, err)
		provider.ClientID = "overflow"
		changed, err = repo.Update(ctx, provider, &provider.Version)
		require.Error(t, err)
		assert.False(t, changed)
		after, err := repo.Get(ctx, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, before, after)
	}
}

func TestClientIdentityRepositoryCanceledObservation(t *testing.T) {
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testProvider(id.NewServiceID())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	cancel()
	_, applied, changed, err := repo.RecordObservedClientIdentity(ctx, provider.ID, "different", ports.ClientIdentityObservation{})
	require.Error(t, err)
	assert.False(t, applied)
	assert.False(t, changed)
	after, err := repo.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestSessionClientRevisionStorageValidation(t *testing.T) {
	session := refreshTestSession("alice@example.com", id.NewServiceID())
	session.UpstreamClientRevision = -1
	err := NewInMemoryUserSessionRepository().Create(context.Background(), session)
	var validation *storage.StorageError
	require.ErrorAs(t, err, &validation)
	assert.Equal(t, storage.ErrorKindValidation, validation.Kind)
}

func TestClientIdentityRepository(t *testing.T) {
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	storagecontracts.ClientIdentityRepository(t, repo, repo)
}
