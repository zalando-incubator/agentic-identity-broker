package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cimdPrivateKeyJWTAuthMethod model.TokenEndpointAuthMethod = "private_key_jwt"

const cimdClientIDPrefix = "https://broker.example.test/.well-known/oauth-client/"

func testCIMDProvider(providerID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      providerID,
		DisplayName:             "CIMD Provider",
		ClientID:                id.ClientID(cimdClientIDPrefix + providerID.String()),
		Secret:                  model.NewAbsentSecret(),
		TokenEndpointAuthMethod: cimdPrivateKeyJWTAuthMethod,
		IssuerURI:               "https://issuer.example.com",
	}
}

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

func TestInMemoryThirdpartyOAuth2ProviderRepository_OwnershipConflictsAreTypedAndAtomic(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	owner := testProvider(id.NewServiceID(), "https://api.example.com/owned")
	require.NoError(t, repo.Create(ctx, owner))

	contender := testProvider(id.NewServiceID(), "https://api.example.com/owned/")
	err := repo.Create(ctx, contender)
	requireStorageKind(t, err, storage.ErrorKindConflict)
	require.ErrorIs(t, err, storage.ErrProtectedResourceOwned)
	_, err = repo.Get(ctx, contender.ID)
	requireStorageKind(t, err, storage.ErrorKindNotFound)

	contender.ProtectedResources = []string{"https://api.example.com/unowned"}
	require.NoError(t, repo.Create(ctx, contender))
	before, err := repo.Get(ctx, contender.ID)
	require.NoError(t, err)
	replacement := before.Copy()
	replacement.DisplayName = "Must not commit"
	replacement.ProtectedResources = []string{"https://api.example.com/owned"}
	err = repo.Update(ctx, replacement, &before.Version)
	requireStorageKind(t, err, storage.ErrorKindConflict)
	require.ErrorIs(t, err, storage.ErrProtectedResourceOwned)
	after, err := repo.Get(ctx, contender.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	duplicateID := testProvider(owner.ID)
	err = repo.Create(ctx, duplicateID)
	requireStorageKind(t, err, storage.ErrorKindConflict)
	assert.False(t, errors.Is(err, storage.ErrProtectedResourceOwned))
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

func TestInMemoryThirdpartyOAuth2ProviderRepository_CIMDProviderRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testCIMDProvider(id.NewServiceID())

	require.NoError(t, repo.Create(ctx, provider))

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, id.ClientID(cimdClientIDPrefix+provider.ID.String()), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_UpdateToCIMDProviderClearsSecret(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	staticProvider := testProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, staticProvider))

	cimdProvider := testCIMDProvider(staticProvider.ID)
	require.NoError(t, repo.Update(ctx, cimdProvider, nil))

	stored, err := repo.Get(ctx, staticProvider.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, id.ClientID(cimdClientIDPrefix+staticProvider.ID.String()), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}

func testDCRProvider(serviceID id.ServiceID, issuer string, method model.TokenEndpointAuthMethod, secret model.Secret) *model.ThirdpartyOAuth2ProviderEntity {
	provider := testProvider(serviceID)
	resourceURL := "https://resource.example.com/" + serviceID.String()
	completedAt := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	provider.ClientID = "shared-dcr-client"
	provider.IssuerURI = issuer
	provider.TokenEndpointAuthMethod = method
	provider.Secret = secret
	provider.Discovery = model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &resourceURL, ClientMethod: model.ClientBootstrapDCR}
	provider.AuthorizationParams = map[string]string{"resource": resourceURL}
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	provider.Endpoints = model.OAuth2Endpoints{AuthorizeEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token"}
	return provider
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_DCRIdentityIsIssuerScoped(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	first := testDCRProvider(id.NewServiceID(), "https://first.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	second := testDCRProvider(id.NewServiceID(), "https://second.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	duplicate := testDCRProvider(id.NewServiceID(), first.IssuerURI, model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	duplicate.DisplayName = "Rejected duplicate"
	requireStorageKind(t, repo.Create(ctx, duplicate), storage.ErrorKindConflict)
	_, err := repo.Get(ctx, duplicate.ID)
	requireStorageKind(t, err, storage.ErrorKindNotFound)

	for _, original := range []*model.ThirdpartyOAuth2ProviderEntity{first, second} {
		stored, err := repo.Get(ctx, original.ID)
		require.NoError(t, err)
		assert.Equal(t, original.IssuerURI, stored.IssuerURI)
		assert.Equal(t, original.ClientID, stored.ClientID)
		assert.Equal(t, original.DisplayName, stored.DisplayName)
		assert.EqualValues(t, 1, stored.Version)
		assert.Equal(t, model.ClientBootstrapDCR, stored.Discovery.ClientMethod)
		assert.True(t, stored.Secret.IsAbsent())
	}

	require.NoError(t, repo.Delete(ctx, first.ID))
	require.NoError(t, repo.Create(ctx, duplicate))
	otherIssuer, err := repo.Get(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, second.IssuerURI, otherIssuer.IssuerURI)
	assert.True(t, otherIssuer.Secret.IsAbsent())
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_DCRDuplicatePreservesConfidentialCredentials(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(NewInMemoryUserSessionRepository())
	firstCiphertext := []byte("first encrypted credential")
	secondCiphertext := []byte("second encrypted credential")
	first := testDCRProvider(id.NewServiceID(), "https://first.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret(firstCiphertext))
	second := testDCRProvider(id.NewServiceID(), "https://second.example.com", model.TokenEndpointAuthMethodClientSecretPost, model.NewEncryptedSecret(secondCiphertext))
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	duplicate := testDCRProvider(id.NewServiceID(), first.IssuerURI, model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret([]byte("replacement credential")))
	createErr := repo.Create(ctx, duplicate)
	requireStorageKind(t, createErr, storage.ErrorKindConflict)
	require.ErrorIs(t, createErr, storage.ErrDuplicateDCRClientIdentity)
	_, err := repo.Get(ctx, duplicate.ID)
	requireStorageKind(t, err, storage.ErrorKindNotFound)

	replacement := first.Copy()
	replacement.IssuerURI = second.IssuerURI
	replacement.Endpoints = second.Endpoints
	replacement.Secret = model.NewEncryptedSecret([]byte("updated credential"))
	completedAt := first.DiscoveryStatus.LastAttemptAt.Add(time.Minute)
	replacement.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	updateErr := repo.Update(ctx, replacement, nil)
	requireStorageKind(t, updateErr, storage.ErrorKindConflict)
	require.ErrorIs(t, updateErr, storage.ErrDuplicateDCRClientIdentity)

	for _, tc := range []struct {
		original   *model.ThirdpartyOAuth2ProviderEntity
		ciphertext []byte
	}{
		{first, firstCiphertext},
		{second, secondCiphertext},
	} {
		stored, err := repo.Get(ctx, tc.original.ID)
		require.NoError(t, err)
		assert.Equal(t, tc.original.IssuerURI, stored.IssuerURI)
		assert.Equal(t, tc.original.DisplayName, stored.DisplayName)
		assert.Equal(t, tc.original.TokenEndpointAuthMethod, stored.TokenEndpointAuthMethod)
		assert.EqualValues(t, 1, stored.Version)
		ciphertext, err := stored.Secret.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, tc.ciphertext, ciphertext)
	}
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_CreateDiscoveryStateIsOwned(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testCIMDProvider(id.NewServiceID())
	resourceURL := "https://api.example.com/discovery"
	completedAt := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	provider.Discovery = model.DiscoveryConfig{
		EnableDiscovery: true,
		ResourceURL:     &resourceURL,
		ClientMethod:    model.ClientBootstrapCIMD,
	}
	provider.AuthorizationParams = map[string]string{"resource": "https://audience.example.com"}
	provider.ResourceExplicit = true
	provider.DiscoveryStatus = model.DiscoveryStatus{
		LastAttemptAt: &completedAt,
		LastSuccessAt: &completedAt,
	}
	require.NoError(t, repo.Create(ctx, provider))

	// The returned state must belong to the repository, not to the caller's entity.
	resourceURL = "https://changed.example.com"
	completedAt = completedAt.Add(time.Hour)
	provider.ResourceExplicit = false
	provider.Discovery.ClientMethod = ""

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.Discovery.ResourceURL)
	assert.Equal(t, "https://api.example.com/discovery", *stored.Discovery.ResourceURL)
	assert.Equal(t, model.ClientBootstrapCIMD, stored.Discovery.ClientMethod)
	assert.True(t, stored.ResourceExplicit)
	assert.Equal(t, "https://audience.example.com", stored.AuthorizationParams["resource"])
	require.NotNil(t, stored.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, stored.DiscoveryStatus.LastSuccessAt)
	initial := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	assert.Equal(t, initial, *stored.DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, initial, *stored.DiscoveryStatus.LastSuccessAt)
	assert.Nil(t, stored.DiscoveryStatus.FailureReason)

	// Get and List must not expose pointers into the owned state or into each other.
	*stored.Discovery.ResourceURL = "https://mutated.example.com"
	*stored.DiscoveryStatus.LastAttemptAt = initial.Add(2 * time.Hour)
	assert.Equal(t, initial, *stored.DiscoveryStatus.LastSuccessAt)
	*stored.DiscoveryStatus.LastSuccessAt = initial.Add(2 * time.Hour)
	listed, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, provider.ID, listed[0].ID)
	require.NotNil(t, listed[0].Discovery.ResourceURL)
	assert.Equal(t, "https://api.example.com/discovery", *listed[0].Discovery.ResourceURL)
	assert.Equal(t, model.ClientBootstrapCIMD, listed[0].Discovery.ClientMethod)
	assert.True(t, listed[0].ResourceExplicit)
	require.NotNil(t, listed[0].DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, listed[0].DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, initial, *listed[0].DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, initial, *listed[0].DiscoveryStatus.LastSuccessAt)

	*listed[0].DiscoveryStatus.LastAttemptAt = initial.Add(3 * time.Hour)
	*listed[0].DiscoveryStatus.LastSuccessAt = initial.Add(3 * time.Hour)
	again, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, again.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, again.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, initial, *again.DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, initial, *again.DiscoveryStatus.LastSuccessAt)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_UpdateCommitsActiveDiscoveryAndStatusTogether(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(NewInMemoryUserSessionRepository())
	provider := testCIMDProvider(id.NewServiceID())
	oldResource := "https://api.example.com/old"
	initial := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	failedAt := initial.Add(time.Minute)
	failure := "authorization_server_metadata_invalid"
	provider.Discovery = model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &oldResource, ClientMethod: model.ClientBootstrapCIMD}
	provider.AuthorizationParams = map[string]string{"resource": "https://audience.example.com"}
	provider.ResourceExplicit = true
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &failedAt, LastSuccessAt: &initial, FailureReason: &failure}
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)

	updated := testCIMDProvider(provider.ID)
	newResource := "https://api.example.com/new"
	succeededAt := initial.Add(2 * time.Minute)
	updated.Discovery = model.DiscoveryConfig{EnableDiscovery: true, ResourceURL: &newResource, ClientMethod: model.ClientBootstrapCIMD}
	updated.AuthorizationParams = map[string]string{"resource": newResource}
	updated.Endpoints.TokenEndpoint = "https://issuer.example.com/new-token"
	updated.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &succeededAt, LastSuccessAt: &succeededAt}
	expectedVersion := int64(1)
	const readers = 16
	start := make(chan struct{})
	observed := make(chan struct {
		provider *model.ThirdpartyOAuth2ProviderEntity
		err      error
	}, readers)
	for range readers {
		go func() {
			<-start
			entity, err := repo.Get(ctx, provider.ID)
			observed <- struct {
				provider *model.ThirdpartyOAuth2ProviderEntity
				err      error
			}{entity, err}
		}()
	}
	close(start)
	require.NoError(t, repo.Update(ctx, updated, &expectedVersion))

	got, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	for range readers {
		result := <-observed
		require.NoError(t, result.err)
		if result.provider.Version == before.Version {
			assert.Equal(t, before, result.provider, "reader saw a partial pre-refresh state")
		} else {
			assert.Equal(t, got, result.provider, "reader saw a partial committed refresh")
		}
	}
	listed, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	for name, stored := range map[string]*model.ThirdpartyOAuth2ProviderEntity{"get": got, "list": listed[0]} {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, stored)
			assert.EqualValues(t, 2, stored.Version)
			require.NotNil(t, stored.Discovery.ResourceURL)
			assert.Equal(t, newResource, *stored.Discovery.ResourceURL)
			assert.Equal(t, model.ClientBootstrapCIMD, stored.Discovery.ClientMethod)
			assert.False(t, stored.ResourceExplicit)
			assert.Equal(t, newResource, stored.AuthorizationParams["resource"])
			assert.Equal(t, "https://issuer.example.com/new-token", stored.Endpoints.TokenEndpoint)
			require.NotNil(t, stored.DiscoveryStatus.LastAttemptAt)
			require.NotNil(t, stored.DiscoveryStatus.LastSuccessAt)
			assert.Equal(t, succeededAt, *stored.DiscoveryStatus.LastAttemptAt)
			assert.Equal(t, succeededAt, *stored.DiscoveryStatus.LastSuccessAt)
			assert.Nil(t, stored.DiscoveryStatus.FailureReason)
		})
	}
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_DiscoveryFailureRetainsActiveStateAndSessions(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	sessions := NewInMemoryUserSessionRepository()
	ciphertext := []byte("encrypted registered client credential")
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret(ciphertext))
	resourceURL := "https://api.example.com/resource"
	succeededAt := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	provider.CreatedAt = succeededAt.Add(-time.Hour)
	provider.UpdatedAt = succeededAt
	provider.Discovery.ResourceURL = &resourceURL
	provider.ResourceExplicit = true
	provider.AuthorizationParams = map[string]string{"resource": "https://audience.example.com"}
	provider.Endpoints.TokenEndpoint = "https://issuer.example.com/token"
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &succeededAt, LastSuccessAt: &succeededAt}
	require.NoError(t, repo.Create(ctx, provider))
	principal := id.Principal("connected-user@example.com")
	session := refreshTestSession(principal, provider.ID)
	require.NoError(t, sessions.Create(ctx, session))

	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	failedAt := succeededAt.Add(time.Minute)
	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt, "authorization_server_metadata_invalid"))

	after, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Version, after.Version)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt)
	assert.Equal(t, before.Discovery, after.Discovery)
	assert.Equal(t, before.ResourceExplicit, after.ResourceExplicit)
	assert.Equal(t, before.AuthorizationParams, after.AuthorizationParams)
	assert.Equal(t, before.ClientID, after.ClientID)
	assert.Equal(t, before.Secret, after.Secret)
	assert.Equal(t, before.TokenEndpointAuthMethod, after.TokenEndpointAuthMethod)
	storedCiphertext, err := after.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, ciphertext, storedCiphertext)
	assert.Equal(t, before.IssuerURI, after.IssuerURI)
	assert.Equal(t, before.Endpoints, after.Endpoints)
	require.NotNil(t, after.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, after.DiscoveryStatus.LastSuccessAt)
	require.NotNil(t, after.DiscoveryStatus.FailureReason)
	assert.Equal(t, failedAt, *after.DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, succeededAt, *after.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, "authorization_server_metadata_invalid", *after.DiscoveryStatus.FailureReason)
	assert.Equal(t, "failed", after.DiscoveryStatus.Status(after.Discovery.ResourceURL))
	connected, err := sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, connected.ID)
	assert.Equal(t, session.EncryptedRefreshToken, connected.EncryptedRefreshToken)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt.Add(time.Second), "invalid?client_secret=leak"), storage.ErrorKindValidation)

	// Neither an obsolete version nor an out-of-order attempt can replace the current status.
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version-1, failedAt.Add(time.Hour), "resource_metadata_invalid"), storage.ErrorKindConflict)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, succeededAt.Add(-time.Second), "resource_metadata_invalid"), storage.ErrorKindConflict)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt, "resource_metadata_invalid"), storage.ErrorKindConflict)
	unchanged, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, after, unchanged)

	// A newer successful update wins over a late failure from a previous version.
	newSuccess := succeededAt.Add(2 * time.Minute)
	refreshed := after.Copy()
	refreshed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &newSuccess, LastSuccessAt: &newSuccess}
	require.NoError(t, repo.Update(ctx, refreshed, &after.Version))
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, after.Version, newSuccess.Add(time.Minute), "resource_metadata_invalid"), storage.ErrorKindConflict)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, refreshed.Version, failedAt, "resource_metadata_invalid"), storage.ErrorKindConflict)
	current, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, current.Version)
	require.NotNil(t, current.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, current.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, newSuccess, *current.DiscoveryStatus.LastAttemptAt)
	assert.Equal(t, newSuccess, *current.DiscoveryStatus.LastSuccessAt)
	assert.Nil(t, current.DiscoveryStatus.FailureReason)
	assert.Equal(t, "ready", current.DiscoveryStatus.Status(current.Discovery.ResourceURL))
	storedCiphertext, err = current.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, ciphertext, storedCiphertext)
	connected, err = sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ID, connected.ID)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ResourceChangeRequiresNoSessions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		pinned  bool
		manual  bool
		expired bool
	}{
		{name: "derived audience with active session"},
		{name: "pinned audience with expired session", pinned: true, expired: true},
		{name: "discovery to manual with changed audience", manual: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			sessions := NewInMemoryUserSessionRepository()
			repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(sessions)
			provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
			const audienceA = "https://audience.example.test/a"
			const audienceB = "https://audience.example.test/b"
			if tc.pinned {
				provider.ResourceExplicit = true
				provider.AuthorizationParams["resource"] = audienceA
			} else {
				resourceURL := audienceA
				provider.Discovery.ResourceURL = &resourceURL
				provider.AuthorizationParams["resource"] = audienceA
			}
			require.NoError(t, repo.Create(ctx, provider))
			principal := id.Principal("resource-change@example.test")
			session := refreshTestSession(principal, provider.ID)
			session.ExpectedIssuerURI = provider.IssuerURI
			session.ExpectedResource = audienceA
			if tc.expired {
				past := time.Now().Add(-time.Hour)
				session.RefreshTokenExpiresAt = &past
			}
			require.NoError(t, sessions.Create(ctx, session))
			before, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)

			updated := before.Copy()
			updated.DisplayName = "Rejected change"
			if tc.manual {
				updated.Discovery = model.DiscoveryConfig{}
				updated.DiscoveryStatus = model.DiscoveryStatus{}
			}
			if !tc.pinned && !tc.manual {
				resourceURL := audienceB
				updated.Discovery.ResourceURL = &resourceURL
				updated.AuthorizationParams["resource"] = audienceB
			} else {
				updated.AuthorizationParams["resource"] = audienceB
			}
			if !tc.manual {
				completedAt := *before.DiscoveryStatus.LastAttemptAt
				completedAt = completedAt.Add(time.Minute)
				updated.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
			}
			err = repo.Update(ctx, updated, &before.Version)
			requireStorageKind(t, err, storage.ErrorKindConflict)
			require.ErrorIs(t, err, storage.ErrResourceChangeHasSessions)
			after, err := repo.Get(ctx, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			storedSession, err := sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
			require.NoError(t, err)
			require.NotNil(t, storedSession)
			assert.Equal(t, session.ID, storedSession.ID)
			assert.Equal(t, session.EncryptedAccessToken, storedSession.EncryptedAccessToken)
			assert.Equal(t, session.EncryptedRefreshToken, storedSession.EncryptedRefreshToken)
		})
	}
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_PinnedAudienceSurvivesDiscoveryURLChangeWithSession(t *testing.T) {
	ctx := context.Background()
	sessions := NewInMemoryUserSessionRepository()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(sessions)
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	const audience = "https://audience.example.test/pinned"
	provider.AuthorizationParams["resource"] = audience
	provider.ResourceExplicit = true
	require.NoError(t, repo.Create(ctx, provider))
	session := refreshTestSession(id.Principal("pinned@example.test"), provider.ID)
	session.ExpectedIssuerURI = provider.IssuerURI
	session.ExpectedResource = audience
	require.NoError(t, sessions.Create(ctx, session))

	updated := provider.Copy()
	resourceURL := "https://resource.example.test/new-location"
	updated.Discovery.ResourceURL = &resourceURL
	updated.AuthorizationParams = nil
	completedAt := updated.DiscoveryStatus.LastAttemptAt.Add(time.Minute)
	updated.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	require.NoError(t, repo.Update(ctx, updated, &provider.Version))
	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, resourceURL, *stored.Discovery.ResourceURL)
	assert.Equal(t, audience, stored.AuthorizationParams["resource"])
	assert.True(t, stored.ResourceExplicit)
	assert.Equal(t, provider.Version+1, stored.Version)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_EqualStoredMicrosecondKeepsFirstFailure(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	failedAt := before.DiscoveryStatus.LastAttemptAt.Add(time.Minute).Truncate(time.Microsecond)
	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt, "resource_metadata_unavailable"))
	failed, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)

	// A distinct nanosecond timestamp still occupies the same persisted microsecond.
	completedAt := failedAt.Add(500 * time.Nanosecond)
	stale := before.Copy()
	resourceURL := "https://resource.example.test/replacement"
	stale.Discovery.ResourceURL = &resourceURL
	stale.AuthorizationParams["resource"] = resourceURL
	stale.Endpoints.TokenEndpoint = "https://issuer.example.com/replacement-token"
	stale.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	requireStorageKind(t, repo.Update(ctx, stale, &before.Version), storage.ErrorKindConflict)
	after, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, failed, after)
	assert.Equal(t, "failed", after.DiscoveryStatus.Status(after.Discovery.ResourceURL))
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_FailureOnlyTieKeepsFirstOutcome(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, provider))
	ready, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	firstAt := ready.DiscoveryStatus.LastAttemptAt.Add(time.Minute).Truncate(time.Microsecond)
	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, firstAt, "resource_metadata_unavailable"))
	first, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, firstAt.Add(500*time.Nanosecond), "issuer_mismatch"), storage.ErrorKindConflict)
	after, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, first, after)
	assert.Equal(t, ready.Version, after.Version)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_StaleSuccessCannotEraseNewerFailure(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(NewInMemoryUserSessionRepository())
	ciphertext := []byte("encrypted registered client credential")
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret(ciphertext))
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, before.DiscoveryStatus.LastAttemptAt)

	// Failure writes do not increment the version. A refresh prepared before the
	// failure must still be unable to replace the later attempt and active client.
	staleSuccessAt := before.DiscoveryStatus.LastAttemptAt.Add(time.Minute)
	failedAt := staleSuccessAt.Add(time.Minute)
	newResource := "https://resource.example.com/updated"
	stale := before.Copy()
	stale.Discovery.ResourceURL = &newResource
	stale.AuthorizationParams = map[string]string{"resource": newResource}
	stale.Endpoints.TokenEndpoint = "https://issuer.example.com/obsolete-token"
	stale.Secret = model.NewEncryptedSecret([]byte("obsolete replacement credential"))
	stale.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &staleSuccessAt, LastSuccessAt: &staleSuccessAt}

	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt, "authorization_server_metadata_invalid"))
	failed, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	requireStorageKind(t, repo.Update(ctx, stale, &before.Version), storage.ErrorKindConflict)
	unchanged, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, failed, unchanged)
	assert.Equal(t, "failed", unchanged.DiscoveryStatus.Status(unchanged.Discovery.ResourceURL))
	storedCiphertext, err := unchanged.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, ciphertext, storedCiphertext)

	// A genuinely later verified refresh still commits its configuration and
	// success status together, clearing only the previous failure.
	laterSuccessAt := failedAt.Add(time.Minute)
	refreshed := failed.Copy()
	refreshed.Discovery.ResourceURL = &newResource
	refreshed.AuthorizationParams = map[string]string{"resource": newResource}
	refreshed.Endpoints.TokenEndpoint = "https://issuer.example.com/current-token"
	refreshed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &laterSuccessAt, LastSuccessAt: &laterSuccessAt}
	require.NoError(t, repo.Update(ctx, refreshed, &failed.Version))
	current, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 2, current.Version)
	assert.Equal(t, "ready", current.DiscoveryStatus.Status(current.Discovery.ResourceURL))
	require.NotNil(t, current.Discovery.ResourceURL)
	assert.Equal(t, newResource, *current.Discovery.ResourceURL)
	assert.Equal(t, newResource, current.AuthorizationParams["resource"])
	assert.Equal(t, refreshed.Endpoints.TokenEndpoint, current.Endpoints.TokenEndpoint)
	assert.Equal(t, before.ClientID, current.ClientID)
	assert.Equal(t, before.TokenEndpointAuthMethod, current.TokenEndpointAuthMethod)
	storedCiphertext, err = current.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, ciphertext, storedCiphertext)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ConcurrentDiscoveryFailuresKeepLatestAttempt(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodNone, model.NewAbsentSecret())
	require.NoError(t, repo.Create(ctx, provider))
	ready, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, ready.DiscoveryStatus.LastSuccessAt)
	olderAt := ready.DiscoveryStatus.LastSuccessAt.Add(time.Minute)
	newerAt := olderAt.Add(time.Minute)

	start := make(chan struct{})
	olderResult := make(chan error, 1)
	newerResult := make(chan error, 1)
	go func() {
		<-start
		olderResult <- repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, olderAt, "resource_metadata_invalid")
	}()
	go func() {
		<-start
		newerResult <- repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, newerAt, "authorization_server_metadata_invalid")
	}()
	close(start)
	if err := <-olderResult; err != nil {
		requireStorageKind(t, err, storage.ErrorKindConflict)
	}
	require.NoError(t, <-newerResult)

	current, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, current.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, current.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, ready.Version, current.Version)
	assert.Equal(t, *ready.DiscoveryStatus.LastSuccessAt, *current.DiscoveryStatus.LastSuccessAt)
	assert.Equal(t, newerAt, *current.DiscoveryStatus.LastAttemptAt)
	require.NotNil(t, current.DiscoveryStatus.FailureReason)
	assert.Equal(t, "authorization_server_metadata_invalid", *current.DiscoveryStatus.FailureReason)
	assert.Equal(t, "failed", current.DiscoveryStatus.Status(current.Discovery.ResourceURL))
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_DeleteRemovesDiscoveryStatus(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	provider := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret([]byte("deleted encrypted credential")))
	resourceURL := "https://api.example.com/resource"
	succeededAt := time.Date(2026, time.October, 7, 10, 0, 0, 0, time.UTC)
	provider.Discovery.ResourceURL = &resourceURL
	provider.AuthorizationParams = map[string]string{"resource": resourceURL}
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &succeededAt, LastSuccessAt: &succeededAt}
	require.NoError(t, repo.Create(ctx, provider))
	active, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, active.DiscoveryStatus.LastSuccessAt)
	require.NoError(t, repo.Delete(ctx, provider.ID))
	_, err = repo.Get(ctx, provider.ID)
	requireStorageKind(t, err, storage.ErrorKindNotFound)
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, active.Version, succeededAt.Add(time.Minute), "resource_metadata_invalid"), storage.ErrorKindNotFound)

	manual := testCIMDProvider(provider.ID)
	require.NoError(t, repo.Create(ctx, manual))
	requireStorageKind(t, repo.RecordDiscoveryFailure(ctx, provider.ID, active.Version, succeededAt.Add(2*time.Minute), "resource_metadata_invalid"), storage.ErrorKindConflict)
	stored, err := repo.Get(ctx, manual.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.Discovery.ResourceURL)
	assert.Empty(t, stored.Discovery.ClientMethod)
	assert.Nil(t, stored.DiscoveryStatus.LastAttemptAt)
	assert.Nil(t, stored.DiscoveryStatus.LastSuccessAt)
	assert.Nil(t, stored.DiscoveryStatus.FailureReason)
	assert.True(t, stored.Secret.IsAbsent(), "deleted DCR credential must not appear in a new service")
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ManualCutoverClearsDiscoveryAudience(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository().WithUserSessionRepository(NewInMemoryUserSessionRepository())
	discovered := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret([]byte("old-sealed-secret")))
	discovered.AuthorizationParams["prompt"] = "consent"
	require.NoError(t, repo.Create(ctx, discovered))

	manual := testProvider(discovered.ID)
	manual.AuthorizationParams = map[string]string{}
	require.NoError(t, repo.Update(ctx, manual, nil))
	stored, err := repo.Get(ctx, discovered.ID)
	require.NoError(t, err)
	assert.Nil(t, stored.Discovery.ResourceURL)
	assert.Nil(t, stored.DiscoveryStatus.LastAttemptAt)
	assert.Empty(t, stored.AuthorizationParams)
}

func TestInMemoryThirdpartyOAuth2ProviderRepository_ManualCutoverKeepsOmittedParamsWithoutDiscoveryMarker(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	discovered := testDCRProvider(id.NewServiceID(), "https://issuer.example.com", model.TokenEndpointAuthMethodClientSecretBasic, model.NewEncryptedSecret([]byte("old-sealed-secret")))
	discovered.ResourceExplicit = true
	discovered.AuthorizationParams["resource"] = "https://audience.example.test/files"
	require.NoError(t, repo.Create(ctx, discovered))

	manual := testProvider(discovered.ID)
	require.NoError(t, repo.Update(ctx, manual, nil))
	stored, err := repo.Get(ctx, discovered.ID)
	require.NoError(t, err)
	assert.Equal(t, discovered.AuthorizationParams, stored.AuthorizationParams)
	assert.False(t, stored.ResourceExplicit)
	assert.Nil(t, stored.Discovery.ResourceURL)
	assert.Nil(t, stored.DiscoveryStatus.LastAttemptAt)
}
