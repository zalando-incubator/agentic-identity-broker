package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testProvider(providerID id.ServiceID, resources ...string) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:                 providerID,
		DisplayName:        "Provider",
		ClientID:           id.ClientID("client-id"),
		Secret:             model.NewEncryptedSecret([]byte("ciphertext")),
		IssuerURI:          "https://issuer.example.com",
		ProtectedResources: resources,
	}
}

func requireStorageKind(t *testing.T, err error, want storage.ErrorKind) {
	t.Helper()
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, want, storageErr.Kind)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_CreateMaterializesResourceSet(t *testing.T) {
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testProvider(id.NewServiceID(), "https://api.example.com/v1/")

	require.NoError(t, repo.Create(context.Background(), provider))
	assert.EqualValues(t, 1, provider.Version)

	stored, err := repo.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stored.Version)
	assert.Equal(t, []string{"https://api.example.com/v1"}, stored.ProtectedResources)

	resolved, err := repo.FindByProtectedResource(context.Background(), "https://api.example.com/v1/")
	require.NoError(t, err)
	assert.Equal(t, provider.ID, resolved.ID)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ProtectedResourceMutations(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, provider))

	added, err := repo.AddProtectedResource(ctx, provider.ID, "https://api.example.com/v1/")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v1", added.Resource)
	assert.Equal(t, []string{"https://api.example.com/v1"}, added.ProtectedResources)
	assert.EqualValues(t, 2, added.Version)
	assert.True(t, added.Changed)

	replayed, err := repo.AddProtectedResource(ctx, provider.ID, "https://api.example.com/v1/")
	require.NoError(t, err)
	assert.False(t, replayed.Changed)
	assert.EqualValues(t, 2, replayed.Version)

	renamed, err := repo.RenameProtectedResource(ctx, provider.ID, "https://api.example.com/v1", "https://api.example.com/v2/")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/v2", renamed.Resource)
	assert.Equal(t, []string{"https://api.example.com/v2"}, renamed.ProtectedResources)
	assert.EqualValues(t, 3, renamed.Version)
	assert.True(t, renamed.Changed)

	removed, err := repo.RemoveProtectedResource(ctx, provider.ID, "https://api.example.com/v2/")
	require.NoError(t, err)
	assert.Empty(t, removed.ProtectedResources)
	assert.EqualValues(t, 4, removed.Version)
	assert.True(t, removed.Changed)

	_, err = repo.RemoveProtectedResource(ctx, provider.ID, "https://api.example.com/v2")
	requireStorageKind(t, err, storage.ErrorKindNotFound)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_RejectsGlobalResourceConflicts(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	first := testProvider(id.NewServiceID(), "https://api.example.com")
	second := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	_, err := repo.AddProtectedResource(ctx, second.ID, "https://api.example.com/")
	requireStorageKind(t, err, storage.ErrorKindConflict)

	_, err = repo.AddProtectedResource(ctx, id.NewServiceID(), "https://new.example.com")
	requireStorageKind(t, err, storage.ErrorKindNotFound)

	_, err = repo.RenameProtectedResource(ctx, second.ID, "https://missing.example.com", "https://new.example.com")
	requireStorageKind(t, err, storage.ErrorKindNotFound)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_UpdateUsesCASOnlyForResourceReplacement(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testProvider(id.NewServiceID(), "https://api.example.com/old")
	require.NoError(t, repo.Create(ctx, provider))

	withoutResources := testProvider(provider.ID, "https://ignored.example.com")
	withoutResources.DisplayName = "Renamed"
	require.NoError(t, repo.Update(ctx, withoutResources, nil))
	assert.EqualValues(t, 2, withoutResources.Version)
	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed", stored.DisplayName)
	assert.Equal(t, []string{"https://api.example.com/old"}, stored.ProtectedResources)

	staleVersion := int64(1)
	replacement := testProvider(provider.ID, "https://api.example.com/new")
	err = repo.Update(ctx, replacement, &staleVersion)
	requireStorageKind(t, err, storage.ErrorKindConflict)

	currentVersion := int64(2)
	require.NoError(t, repo.Update(ctx, replacement, &currentVersion))
	assert.EqualValues(t, 3, replacement.Version)
	stored, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://api.example.com/new"}, stored.ProtectedResources)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ConcurrentClaimsPreserveOwnership(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	first := testProvider(id.NewServiceID())
	second := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, providerID := range []id.ServiceID{first.ID, second.ID} {
		wg.Add(1)
		go func(providerID id.ServiceID) {
			defer wg.Done()
			_, err := repo.AddProtectedResource(ctx, providerID, "https://race.example.com/")
			errs <- err
		}(providerID)
	}
	wg.Wait()
	close(errs)

	successes := 0
	conflicts := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		var storageErr *storage.StorageError
		require.True(t, errors.As(err, &storageErr))
		if storageErr.Kind == storage.ErrorKindConflict {
			conflicts++
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, conflicts)

	differentErrs := make(chan error, 2)
	for _, providerID := range []id.ServiceID{first.ID, second.ID} {
		wg.Add(1)
		go func(providerID id.ServiceID) {
			defer wg.Done()
			_, err := repo.AddProtectedResource(ctx, providerID, "https://different-"+providerID.String()+".example.com")
			differentErrs <- err
		}(providerID)
	}
	wg.Wait()
	close(differentErrs)
	for err := range differentErrs {
		require.NoError(t, err)
	}
	firstResources, firstVersion, err := repo.ListProtectedResources(ctx, first.ID)
	require.NoError(t, err)
	secondResources, secondVersion, err := repo.ListProtectedResources(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, len(firstResources)+len(secondResources))
	assert.EqualValues(t, 5, firstVersion+secondVersion)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ChildResourceResolverLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, provider))

	const resource = "https://api.example.com/resolver"
	_, err := repo.AddProtectedResource(ctx, provider.ID, resource)
	require.NoError(t, err)
	resolved, err := repo.FindByProtectedResource(ctx, resource)
	require.NoError(t, err)
	assert.Equal(t, provider.ID, resolved.ID)

	_, err = repo.RemoveProtectedResource(ctx, provider.ID, resource)
	require.NoError(t, err)
	_, err = repo.FindByProtectedResource(ctx, resource)
	require.Error(t, err)
	assert.True(t, tokenexchange.IsResourceNotConfigured(err))
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_PublicProviderRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      id.NewServiceID(),
		DisplayName:             "Public Provider",
		ClientID:                id.ClientID("public-client-id"),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
		IssuerURI:               "https://issuer.example.com",
	}

	require.NoError(t, repo.Create(ctx, provider))

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, model.TokenEndpointAuthMethodNone, stored.TokenEndpointAuthMethod)
	assert.True(t, stored.IsPublicClient())
	assert.True(t, stored.Secret.IsAbsent())
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_RejectsInvalidClientAuthentication(t *testing.T) {
	tests := []struct {
		name   string
		method model.TokenEndpointAuthMethod
		secret model.Secret
	}{
		{
			name:   "confidential service without an encrypted secret",
			secret: model.NewAbsentSecret(),
		},
		{
			name:   "public service with an encrypted secret",
			method: model.TokenEndpointAuthMethodNone,
			secret: model.NewEncryptedSecret([]byte("ciphertext")),
		},
		{
			name:   "service with an unsupported authentication method",
			method: "client_secret_post",
			secret: model.NewEncryptedSecret([]byte("ciphertext")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
			provider := testProvider(id.NewServiceID())
			provider.TokenEndpointAuthMethod = tt.method
			provider.Secret = tt.secret

			err := repo.Create(context.Background(), provider)
			requireStorageKind(t, err, storage.ErrorKindValidation)
		})
	}
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_RejectsPlaintextSecret(t *testing.T) {
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Confidential Provider",
		ClientID:    id.ClientID("confidential-client-id"),
		Secret:      model.NewPlaintextSecret("plaintext-secret"),
		IssuerURI:   "https://issuer.example.com",
	}

	err := repo.Create(context.Background(), provider)
	requireStorageKind(t, err, storage.ErrorKindValidation)
}
