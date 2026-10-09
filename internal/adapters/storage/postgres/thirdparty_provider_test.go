//go:build integration
// +build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

const (
	cimdPrivateKeyJWTAuthMethod model.TokenEndpointAuthMethod = "private_key_jwt"
	cimdClientIDPrefix                                        = "https://broker.example.test/.well-known/oauth-client/"
)

func testCIMDProvider(providerID id.ServiceID) *model.ThirdpartyOAuth2ProviderEntity {
	provider := newTestEntity()
	provider.ID = providerID
	provider.ClientID = id.ClientID(cimdClientIDPrefix + providerID.String())
	provider.Secret = model.NewAbsentSecret()
	provider.TokenEndpointAuthMethod = cimdPrivateKeyJWTAuthMethod
	return provider
}

func TestPostgresThirdpartyOAuth2ProviderRepository_ProtectedResources(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil

	require.NoError(t, repo.Create(ctx, provider))
	assert.Equal(t, int64(1), provider.Version)

	resources, version, err := repo.ListProtectedResources(ctx, provider.ID)
	require.NoError(t, err)
	assert.Empty(t, resources)
	assert.Equal(t, int64(1), version)

	added, err := repo.AddProtectedResource(ctx, provider.ID, "https://api.example.com/added")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/added", added.Resource)
	assert.Equal(t, []string{"https://api.example.com/added"}, added.ProtectedResources)
	assert.Equal(t, int64(2), added.Version)
	assert.True(t, added.Changed)

	resources, version, err = repo.ListProtectedResources(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, added.ProtectedResources, resources)
	assert.Equal(t, added.Version, version)

	replay, err := repo.AddProtectedResource(ctx, provider.ID, "https://api.example.com/added")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/added", replay.Resource)
	assert.Equal(t, []string{"https://api.example.com/added"}, replay.ProtectedResources)
	assert.Equal(t, int64(2), replay.Version)
	assert.False(t, replay.Changed)

	renamed, err := repo.RenameProtectedResource(ctx, provider.ID, "https://api.example.com/added", "https://api.example.com/renamed")
	require.NoError(t, err)
	assert.Equal(t, "https://api.example.com/renamed", renamed.Resource)
	assert.Equal(t, []string{"https://api.example.com/renamed"}, renamed.ProtectedResources)
	assert.Equal(t, int64(3), renamed.Version)
	assert.True(t, renamed.Changed)

	other := newTestEntity()
	other.ID = id.NewServiceID()
	other.ProtectedResources = nil
	require.NoError(t, repo.Create(ctx, other))

	_, err = repo.AddProtectedResource(ctx, other.ID, "https://api.example.com/renamed")
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

	removed, err := repo.RemoveProtectedResource(ctx, provider.ID, "https://api.example.com/renamed")
	require.NoError(t, err)
	assert.Empty(t, removed.ProtectedResources)
	assert.Equal(t, int64(4), removed.Version)
	assert.True(t, removed.Changed)

	resources, version, err = repo.ListProtectedResources(ctx, provider.ID)
	require.NoError(t, err)
	assert.Empty(t, resources)
	assert.Equal(t, removed.Version, version)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_OwnershipConflictsAreTypedAndAtomic(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)

	owner := newTestEntity()
	owner.ID = id.NewServiceID()
	owner.ProtectedResources = []string{"https://api.example.com/owned"}
	require.NoError(t, repo.Create(ctx, owner))
	contender := newTestEntity()
	contender.ID = id.NewServiceID()
	contender.ProtectedResources = []string{"https://api.example.com/owned"}
	err := repo.Create(ctx, contender)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	require.ErrorIs(t, err, storage.ErrProtectedResourceOwned)
	_, err = repo.Get(ctx, contender.ID)
	require.ErrorIs(t, err, ports.ErrNotFound)

	contender.ProtectedResources = []string{"https://api.example.com/unowned"}
	require.NoError(t, repo.Create(ctx, contender))
	before, err := repo.Get(ctx, contender.ID)
	require.NoError(t, err)
	replacement := before.Copy()
	replacement.DisplayName = "Must not commit"
	replacement.ProtectedResources = []string{"https://api.example.com/owned"}
	err = repo.Update(ctx, replacement, &before.Version)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	require.ErrorIs(t, err, storage.ErrProtectedResourceOwned)
	after, err := repo.Get(ctx, contender.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)

	duplicateID := newTestEntity()
	duplicateID.ID = owner.ID
	duplicateID.ProtectedResources = nil
	err = repo.Create(ctx, duplicateID)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.False(t, errors.Is(err, storage.ErrProtectedResourceOwned))
}

func TestPostgresThirdpartyOAuth2ProviderRepository_UpdateCAS(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = []string{"https://api.example.com/original"}
	require.NoError(t, repo.Create(ctx, provider))

	expectedVersion := provider.Version
	provider.DisplayName = "Updated Provider"
	provider.ProtectedResources = []string{"https://api.example.com/replaced"}
	require.NoError(t, repo.Update(ctx, provider, &expectedVersion))
	assert.Equal(t, int64(2), provider.Version)

	resources, version, err := repo.ListProtectedResources(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://api.example.com/replaced"}, resources)
	assert.Equal(t, provider.Version, version)

	staleVersion := expectedVersion
	provider.ProtectedResources = []string{"https://api.example.com/stale"}
	err = repo.Update(ctx, provider, &staleVersion)
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.True(t, errors.As(err, &storageErr))
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

	provider.ProtectedResources = []string{"https://api.example.com/ignored"}
	require.NoError(t, repo.Update(ctx, provider, nil))
	resources, version, err = repo.ListProtectedResources(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://api.example.com/replaced"}, resources)
	assert.Equal(t, int64(3), version)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_GetUsesChildResourcesAndEncryptedSecret(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = []string{"https://api.example.com/child"}
	require.NoError(t, repo.Create(ctx, provider))

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, provider.ProtectedResources, stored.ProtectedResources)
	assert.Equal(t, int64(1), stored.Version)
	assert.True(t, stored.Secret.IsEncrypted())
	ciphertext, err := stored.Secret.GetCiphertext()
	require.NoError(t, err)
	assert.Equal(t, testCiphertext, ciphertext)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_ChildResourceResolverLifecycle(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
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

func TestPostgresThirdpartyOAuth2ProviderRepository_UpdatePreservesCanonicalID(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	ctx := context.Background()
	canonicalID := "preserved-canonical-service"
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.CanonicalID = &canonicalID
	require.NoError(t, repo.Create(ctx, provider))

	updated := *provider
	updated.CanonicalID = nil
	updated.DisplayName = "Updated Provider"
	updated.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.Update(ctx, &updated, nil))
	require.NotNil(t, updated.CanonicalID)
	assert.Equal(t, canonicalID, *updated.CanonicalID)

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.CanonicalID)
	assert.Equal(t, canonicalID, *stored.CanonicalID)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DeleteUnreferencedProvider(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
	require.NoError(t, repo.Create(context.Background(), provider))

	require.NoError(t, repo.Delete(context.Background(), provider.ID))
	_, err := repo.Get(context.Background(), provider.ID)
	require.ErrorIs(t, err, ports.ErrNotFound)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DeleteProviderReferencedByActiveGrant(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
	require.NoError(t, repo.Create(ctx, provider))

	agent := createUserGrantTestAgent(t, NewAgentRepository(adapter), "provider-reference")
	grant := newUserGrant(
		id.Principal("grant-user@example.com"),
		agent.ID,
		newGrantedPermissionSetEntry(t, adapter, id.NewServiceID(), provider.ID),
	)
	require.NoError(t, NewUserGrantRepository(adapter).Create(ctx, grant))

	err := repo.Delete(ctx, provider.ID)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Contains(t, storageErr.Error(), "user grant")
	_, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)

	tx, err := adapter.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`)
	require.NoError(t, err)
	rows, err := tx.QueryContext(ctx, `EXPLAIN `+activeProviderGrantReferenceQuery, provider.ID.String())
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var plan strings.Builder
	for rows.Next() {
		var step string
		require.NoError(t, rows.Scan(&step))
		plan.WriteString(step)
		plan.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	assert.Contains(t, plan.String(), "idx_grants_permission_sets")
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DeleteProviderWithSessionReturnsConflict(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
	require.NoError(t, repo.Create(ctx, provider))

	now := time.Now().UTC()
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal("session-user@example.com"),
		ServiceID:            provider.ID,
		EncryptedAccessToken: []byte("encrypted-token"),
		TokenType:            "Bearer",
		Scope:                []string{"read"},
		EncryptionContext:    storage.EncryptionContext{ServiceID: provider.ID},
		InitiatedAt:          now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	require.NoError(t, NewUserSessionRepository(adapter).Create(ctx, session))

	err := repo.Delete(ctx, provider.ID)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	_, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DeleteProviderReferencedByPermissionSet(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
	require.NoError(t, repo.Create(ctx, provider))

	now := time.Now().UTC()
	permissionSet := &storage.PermissionSet{
		ID:          id.NewPermissionSetID(),
		Name:        "Provider reference permission set",
		Description: "Prevents provider deletion while referenced",
		ServiceScopes: []storage.ServiceScope{{
			ServiceID:       provider.ID,
			Scopes:          []string{"read"},
			RequirementType: storage.RequirementTypeMandatory,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, NewPermissionSetRepository(adapter).Create(ctx, permissionSet))

	err := repo.Delete(ctx, provider.ID)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	_, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DeleteProviderReferencedByAgent(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := newTestEntity()
	provider.ID = id.NewServiceID()
	provider.ProtectedResources = nil
	require.NoError(t, repo.Create(ctx, provider))

	now := time.Now().UTC()
	clientID := id.ClientID("provider-reference-agent")
	agent := &storage.Agent{
		ClientID:    &clientID,
		DisplayName: "Provider Reference Agent",
		Description: "Prevents provider deletion while referenced",
		ServiceRequirements: []storage.ServiceRequirement{{
			ServiceID:       provider.ID,
			RequirementType: storage.RequirementTypeMandatory,
			RequiredScopes:  []string{"read"},
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	attachTestPermissionSet(t, adapter, agent)
	require.NoError(t, NewAgentRepository(adapter).Create(ctx, agent))

	err := repo.Delete(ctx, provider.ID)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	_, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_CIMDProviderRoundTrip(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	provider := testCIMDProvider(id.NewServiceID())
	require.NoError(t, repo.Create(ctx, provider))

	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, id.ClientID(cimdClientIDPrefix+provider.ID.String()), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}

func TestPostgresThirdpartyOAuth2ProviderRepository_UpdateToCIMDProviderClearsSecret(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	staticProvider := newTestEntity()
	staticProvider.ID = id.NewServiceID()
	require.NoError(t, repo.Create(ctx, staticProvider))

	cimdProvider := testCIMDProvider(staticProvider.ID)
	require.NoError(t, repo.Update(ctx, cimdProvider, nil))

	stored, err := repo.Get(ctx, staticProvider.ID)
	require.NoError(t, err)
	assert.Equal(t, cimdPrivateKeyJWTAuthMethod, stored.TokenEndpointAuthMethod)
	assert.Equal(t, id.ClientID(cimdClientIDPrefix+staticProvider.ID.String()), stored.ClientID)
	assert.True(t, stored.Secret.IsAbsent())
}

func discoveredCIMDProvider(providerID id.ServiceID, resourceURL string, completedAt time.Time) *model.ThirdpartyOAuth2ProviderEntity {
	provider := testCIMDProvider(providerID)
	provider.Discovery.MetadataURL = nil
	provider.Discovery.ResourceURL = &resourceURL
	provider.Discovery.ClientMethod = model.ClientBootstrapCIMD
	provider.AuthorizationParams = map[string]string{"resource": resourceURL}
	provider.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	provider.ProtectedResources = nil
	return provider
}

type discoveryProviderRow struct {
	ResourceURL       string
	ClientMethod      string
	ResourceExplicit  bool
	EffectiveResource string
	ClientID          string
	SecretCiphertext  []byte
	TokenAuthMethod   string
	IssuerURI         string
	TokenEndpoint     string
	AuthorizeEndpoint string
	LastAttemptAt     time.Time
	LastSuccessAt     time.Time
	FailureReason     sql.NullString
	UpdatedAt         time.Time
	Version           int64
}

func readDiscoveryProviderRow(t *testing.T, adapter *Adapter, serviceID id.ServiceID) discoveryProviderRow {
	t.Helper()
	var row discoveryProviderRow
	err := adapter.db.QueryRowContext(context.Background(), `SELECT resource_url, client_method, resource_explicit,
		authorization_params ->> 'resource', client_id, client_secret_encrypted, token_endpoint_auth_method,
		issuer_uri, token_endpoint, authorize_endpoint, discovery_last_attempt_at, discovery_last_success_at,
		discovery_failure_reason, updated_at, version
		FROM thirdparty_oauth2_services WHERE id = $1`, serviceID).Scan(
		&row.ResourceURL, &row.ClientMethod, &row.ResourceExplicit, &row.EffectiveResource,
		&row.ClientID, &row.SecretCiphertext, &row.TokenAuthMethod, &row.IssuerURI,
		&row.TokenEndpoint, &row.AuthorizeEndpoint, &row.LastAttemptAt, &row.LastSuccessAt,
		&row.FailureReason, &row.UpdatedAt, &row.Version,
	)
	require.NoError(t, err)
	row.LastAttemptAt = row.LastAttemptAt.UTC()
	row.LastSuccessAt = row.LastSuccessAt.UTC()
	return row
}

func assertDiscoveryInstant(t *testing.T, want time.Time, got *time.Time) {
	t.Helper()
	require.NotNil(t, got)
	assert.WithinDuration(t, want, *got, 0)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DiscoveryCreateAndRefreshAreAtomic(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	initialSuccess := time.Date(2026, 4, 17, 14, 20, 0, 0, time.UTC)
	const initialResource = "https://resource.example.test/v1"
	provider := discoveredCIMDProvider(id.NewServiceID(), initialResource, initialSuccess)
	require.NoError(t, repo.Create(ctx, provider))

	created := readDiscoveryProviderRow(t, adapter, provider.ID)
	assert.Equal(t, initialResource, created.ResourceURL)
	assert.Equal(t, string(model.ClientBootstrapCIMD), created.ClientMethod)
	assert.False(t, created.ResourceExplicit)
	assert.Equal(t, initialResource, created.EffectiveResource)
	assert.Equal(t, provider.ClientID.String(), created.ClientID)
	assert.Nil(t, created.SecretCiphertext)
	assert.Equal(t, string(cimdPrivateKeyJWTAuthMethod), created.TokenAuthMethod)
	assert.Equal(t, initialSuccess, created.LastAttemptAt)
	assert.Equal(t, initialSuccess, created.LastSuccessAt)
	assert.False(t, created.FailureReason.Valid)
	assert.Equal(t, int64(1), created.Version)

	restartedRepo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	stored, err := restartedRepo.Get(ctx, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, stored.Discovery.ResourceURL)
	assert.Equal(t, initialResource, *stored.Discovery.ResourceURL)
	assert.Nil(t, stored.Discovery.MetadataURL)
	assert.Equal(t, model.ClientBootstrapCIMD, stored.Discovery.ClientMethod)
	assert.Equal(t, initialResource, stored.AuthorizationParams["resource"])
	assertDiscoveryInstant(t, initialSuccess, stored.DiscoveryStatus.LastAttemptAt)
	assertDiscoveryInstant(t, initialSuccess, stored.DiscoveryStatus.LastSuccessAt)
	assert.Nil(t, stored.DiscoveryStatus.FailureReason)
	assert.Empty(t, stored.ProtectedResources)

	refreshedAt := initialSuccess.Add(time.Hour)
	const nextResource = "https://resource.example.test/v2"
	const overrideResource = "https://audience.example.test/custom"
	refreshedResourceURL := nextResource
	stored.Discovery.ResourceURL = &refreshedResourceURL
	stored.AuthorizationParams = map[string]string{"resource": overrideResource}
	stored.ResourceExplicit = true
	stored.Endpoints.TokenEndpoint = "https://example.com/next-token"
	stored.Endpoints.AuthorizeEndpoint = "https://example.com/next-authorize"
	stored.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &refreshedAt, LastSuccessAt: &refreshedAt}
	stored.UpdatedAt = refreshedAt
	expectedVersion := stored.Version
	require.NoError(t, restartedRepo.Update(ctx, stored, &expectedVersion))

	refreshed := readDiscoveryProviderRow(t, adapter, provider.ID)
	assert.Equal(t, nextResource, refreshed.ResourceURL)
	assert.Equal(t, string(model.ClientBootstrapCIMD), refreshed.ClientMethod)
	assert.True(t, refreshed.ResourceExplicit)
	assert.Equal(t, overrideResource, refreshed.EffectiveResource)
	assert.Equal(t, "https://example.com/next-token", refreshed.TokenEndpoint)
	assert.Equal(t, "https://example.com/next-authorize", refreshed.AuthorizeEndpoint)
	assert.Equal(t, refreshedAt, refreshed.LastAttemptAt)
	assert.Equal(t, refreshedAt, refreshed.LastSuccessAt)
	assert.False(t, refreshed.FailureReason.Valid)
	assert.Nil(t, refreshed.SecretCiphertext)
	assert.Equal(t, int64(2), refreshed.Version)

	restartedRepo = NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	fromGet, err := restartedRepo.Get(ctx, provider.ID)
	require.NoError(t, err)
	listed, err := restartedRepo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	for _, entity := range []*model.ThirdpartyOAuth2ProviderEntity{fromGet, listed[0]} {
		assert.Equal(t, provider.ID, entity.ID)
		require.NotNil(t, entity.Discovery.ResourceURL)
		assert.Equal(t, nextResource, *entity.Discovery.ResourceURL)
		assert.Equal(t, model.ClientBootstrapCIMD, entity.Discovery.ClientMethod)
		assert.True(t, entity.ResourceExplicit)
		assert.Equal(t, overrideResource, entity.AuthorizationParams["resource"])
		assertDiscoveryInstant(t, refreshedAt, entity.DiscoveryStatus.LastAttemptAt)
		assertDiscoveryInstant(t, refreshedAt, entity.DiscoveryStatus.LastSuccessAt)
		assert.Nil(t, entity.DiscoveryStatus.FailureReason)
		assert.Equal(t, refreshed.TokenEndpoint, entity.Endpoints.TokenEndpoint)
		assert.True(t, entity.Secret.IsAbsent())
		assert.Equal(t, refreshed.Version, entity.Version)
	}
}

func TestPostgresThirdpartyOAuth2ProviderRepository_DiscoverySuccessReplacesManualCiphertext(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	manual := newTestEntity()
	manual.ID = id.NewServiceID()
	require.NoError(t, repo.Create(ctx, manual))

	var ciphertext []byte
	var resourceURL, clientMethod sql.NullString
	var lastAttemptAt, lastSuccessAt sql.NullTime
	var failureReason sql.NullString
	err := adapter.db.QueryRowContext(ctx, `SELECT client_secret_encrypted, resource_url, client_method,
		discovery_last_attempt_at, discovery_last_success_at, discovery_failure_reason
		FROM thirdparty_oauth2_services WHERE id = $1`, manual.ID).Scan(
		&ciphertext, &resourceURL, &clientMethod, &lastAttemptAt, &lastSuccessAt, &failureReason,
	)
	require.NoError(t, err)
	assert.Equal(t, testCiphertext, ciphertext)
	assert.False(t, resourceURL.Valid)
	assert.False(t, clientMethod.Valid)
	assert.False(t, lastAttemptAt.Valid)
	assert.False(t, lastSuccessAt.Valid)
	assert.False(t, failureReason.Valid)

	completedAt := time.Date(2026, 4, 18, 10, 0, 0, 0, time.UTC)
	const resource = "https://resource.example.test/converted"
	discovered := discoveredCIMDProvider(manual.ID, resource, completedAt)
	discovered.UpdatedAt = completedAt
	expectedVersion := manual.Version
	require.NoError(t, repo.Update(ctx, discovered, &expectedVersion))

	row := readDiscoveryProviderRow(t, adapter, manual.ID)
	assert.Equal(t, resource, row.ResourceURL)
	assert.Equal(t, resource, row.EffectiveResource)
	assert.Equal(t, string(model.ClientBootstrapCIMD), row.ClientMethod)
	assert.Equal(t, string(cimdPrivateKeyJWTAuthMethod), row.TokenAuthMethod)
	assert.Nil(t, row.SecretCiphertext)
	assert.Equal(t, completedAt, row.LastAttemptAt)
	assert.Equal(t, completedAt, row.LastSuccessAt)
	assert.Equal(t, int64(2), row.Version)

	stored, err := NewPostgresThirdpartyOAuth2ProviderRepository(adapter).Get(ctx, manual.ID)
	require.NoError(t, err)
	assert.True(t, stored.Secret.IsAbsent())
	assertDiscoveryInstant(t, completedAt, stored.DiscoveryStatus.LastSuccessAt)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_RecordDiscoveryFailureGuardsActiveState(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()

	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	initialSuccess := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	provider := discoveredCIMDProvider(id.NewServiceID(), "https://resource.example.test/guarded", initialSuccess)
	require.NoError(t, repo.Create(ctx, provider))
	ready := readDiscoveryProviderRow(t, adapter, provider.ID)

	failedAt := initialSuccess.Add(time.Hour)
	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, failedAt, "metadata_unavailable"))
	failed := readDiscoveryProviderRow(t, adapter, provider.ID)
	expectedFailed := ready
	expectedFailed.LastAttemptAt = failedAt
	expectedFailed.FailureReason = sql.NullString{String: "metadata_unavailable", Valid: true}
	assert.Equal(t, expectedFailed, failed)

	stored, err := NewPostgresThirdpartyOAuth2ProviderRepository(adapter).Get(ctx, provider.ID)
	require.NoError(t, err)
	assertDiscoveryInstant(t, failedAt, stored.DiscoveryStatus.LastAttemptAt)
	assertDiscoveryInstant(t, initialSuccess, stored.DiscoveryStatus.LastSuccessAt)
	require.NotNil(t, stored.DiscoveryStatus.FailureReason)
	assert.Equal(t, "metadata_unavailable", *stored.DiscoveryStatus.FailureReason)
	assert.Equal(t, ready.Version, stored.Version)

	for _, attempt := range []time.Time{failedAt, failedAt.Add(-time.Minute)} {
		err := repo.RecordDiscoveryFailure(ctx, provider.ID, ready.Version, attempt, "late_failure")
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		assert.Equal(t, failed, readDiscoveryProviderRow(t, adapter, provider.ID))
	}

	newSuccess := initialSuccess.Add(3 * time.Hour)
	stored.Endpoints.TokenEndpoint = "https://example.com/refreshed-token"
	stored.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &newSuccess, LastSuccessAt: &newSuccess}
	stored.UpdatedAt = newSuccess
	expectedVersion := stored.Version
	require.NoError(t, repo.Update(ctx, stored, &expectedVersion))
	refreshed := readDiscoveryProviderRow(t, adapter, provider.ID)
	assert.Equal(t, int64(2), refreshed.Version)
	assert.Equal(t, "https://example.com/refreshed-token", refreshed.TokenEndpoint)
	assert.Equal(t, newSuccess, refreshed.LastAttemptAt)
	assert.Equal(t, newSuccess, refreshed.LastSuccessAt)
	assert.False(t, refreshed.FailureReason.Valid)

	for _, attempt := range []struct {
		version int64
		at      time.Time
	}{
		{version: refreshed.Version, at: initialSuccess.Add(2 * time.Hour)},
		{version: refreshed.Version, at: newSuccess},
		{version: ready.Version, at: newSuccess.Add(time.Hour)},
	} {
		err := repo.RecordDiscoveryFailure(ctx, provider.ID, attempt.version, attempt.at, "stale_failure")
		var storageErr *storage.StorageError
		require.ErrorAs(t, err, &storageErr)
		assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
		assert.Equal(t, refreshed, readDiscoveryProviderRow(t, adapter, provider.ID))
	}
}

func TestPostgresThirdpartyOAuth2ProviderRepository_ResourceChangeRequiresNoSessions(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	sessions := NewUserSessionRepository(adapter)

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
			const audienceA = "https://audience.example.test/a"
			const audienceB = "https://audience.example.test/b"
			initialAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
			provider := discoveredCIMDProvider(id.NewServiceID(), audienceA, initialAt)
			if tc.pinned {
				resourceURL := "https://resource.example.test/discovery"
				provider.Discovery.ResourceURL = &resourceURL
				provider.AuthorizationParams["resource"] = audienceA
				provider.ResourceExplicit = true
			}
			require.NoError(t, repo.Create(ctx, provider))
			principal := id.Principal("resource-change-" + provider.ID.String() + "@example.test")
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: principal, ServiceID: provider.ID,
				EncryptedAccessToken: []byte("sealed-access"), EncryptedRefreshToken: []byte("sealed-refresh"),
				TokenType: "Bearer", Scope: []string{"read"},
				EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
				InitiatedAt:       initialAt, CreatedAt: initialAt, UpdatedAt: initialAt,
				ExpectedIssuerURI: provider.IssuerURI, ExpectedResource: audienceA,
			}
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
				completedAt := initialAt.Add(time.Minute)
				updated.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
			}
			err = repo.Update(ctx, updated, &before.Version)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
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

func TestPostgresThirdpartyOAuth2ProviderRepository_PinnedAudienceSurvivesDiscoveryURLChangeWithSession(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	sessions := NewUserSessionRepository(adapter)
	initialAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	provider := discoveredCIMDProvider(id.NewServiceID(), "https://resource.example.test/original", initialAt)
	const audience = "https://audience.example.test/pinned"
	provider.AuthorizationParams["resource"] = audience
	provider.ResourceExplicit = true
	require.NoError(t, repo.Create(ctx, provider))
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: id.Principal("pinned@example.test"), ServiceID: provider.ID,
		EncryptedAccessToken: []byte("sealed-access"), TokenType: "Bearer", Scope: []string{"read"},
		EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
		InitiatedAt:       initialAt, CreatedAt: initialAt, UpdatedAt: initialAt,
		ExpectedIssuerURI: provider.IssuerURI, ExpectedResource: audience,
	}
	require.NoError(t, sessions.Create(ctx, session))

	updated := provider.Copy()
	resourceURL := "https://resource.example.test/new-location"
	updated.Discovery.ResourceURL = &resourceURL
	updated.AuthorizationParams = nil
	completedAt := initialAt.Add(time.Minute)
	updated.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &completedAt, LastSuccessAt: &completedAt}
	require.NoError(t, repo.Update(ctx, updated, &provider.Version))
	stored, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, resourceURL, *stored.Discovery.ResourceURL)
	assert.Equal(t, audience, stored.AuthorizationParams["resource"])
	assert.True(t, stored.ResourceExplicit)
	assert.Equal(t, provider.Version+1, stored.Version)
}

func TestPostgresThirdpartyOAuth2ProviderRepository_EqualStoredMicrosecondKeepsFirstFailure(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	initialAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	provider := discoveredCIMDProvider(id.NewServiceID(), "https://resource.example.test/original", initialAt)
	require.NoError(t, repo.Create(ctx, provider))
	before, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	failedAt := initialAt.Add(time.Minute)
	require.NoError(t, repo.RecordDiscoveryFailure(ctx, provider.ID, before.Version, failedAt, "resource_metadata_unavailable"))
	failed := readDiscoveryProviderRow(t, adapter, provider.ID)

	stale := before.Copy()
	resourceURL := "https://resource.example.test/replacement"
	stale.Discovery.ResourceURL = &resourceURL
	stale.AuthorizationParams["resource"] = resourceURL
	stale.Endpoints.TokenEndpoint = "https://example.com/replacement-token"
	stale.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &failedAt, LastSuccessAt: &failedAt}
	stale.UpdatedAt = failedAt
	err = repo.Update(ctx, stale, &before.Version)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Equal(t, failed, readDiscoveryProviderRow(t, adapter, provider.ID))
	after, err := repo.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "failed", after.DiscoveryStatus.Status(after.Discovery.ResourceURL))
}

func TestPostgresUserSessionRepository_StaleAudienceCannotInsertOrReplaceSession(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	repo := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	sessions := NewUserSessionRepository(adapter)
	initialAt := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	const audienceA = "https://resource.example.test/a"
	const audienceB = "https://resource.example.test/b"
	provider := discoveredCIMDProvider(id.NewServiceID(), audienceA, initialAt)
	require.NoError(t, repo.Create(ctx, provider))
	principal := id.Principal("callback@example.test")
	stale := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: provider.ID,
		EncryptedAccessToken: []byte("stale-access"), EncryptedRefreshToken: []byte("stale-refresh"),
		TokenType: "Bearer", Scope: []string{"read"},
		EncryptionContext: storage.EncryptionContext{ServiceID: provider.ID},
		InitiatedAt:       initialAt, CreatedAt: initialAt, UpdatedAt: initialAt,
		ExpectedIssuerURI: provider.IssuerURI, ExpectedResource: audienceA,
	}
	changed := provider.Copy()
	resourceB := audienceB
	changed.Discovery.ResourceURL = &resourceB
	changed.AuthorizationParams["resource"] = audienceB
	committedAt := initialAt.Add(time.Minute)
	changed.DiscoveryStatus = model.DiscoveryStatus{LastAttemptAt: &committedAt, LastSuccessAt: &committedAt}
	require.NoError(t, repo.Update(ctx, changed, &provider.Version))

	err := sessions.Create(ctx, stale)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	count, err := sessions.CountByService(ctx, provider.ID)
	require.NoError(t, err)
	assert.Zero(t, count)

	current := *stale
	current.ExpectedResource = audienceB
	current.EncryptedAccessToken = []byte("current-access")
	current.EncryptedRefreshToken = []byte("current-refresh")
	require.NoError(t, sessions.Create(ctx, &current))
	err = sessions.Create(ctx, stale)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	stored, err := sessions.FindByPrincipalAndService(ctx, principal, provider.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, current.ID, stored.ID)
	assert.Equal(t, current.EncryptedAccessToken, stored.EncryptedAccessToken)
	assert.Equal(t, current.EncryptedRefreshToken, stored.EncryptedRefreshToken)
	assert.Empty(t, stored.ExpectedIssuerURI)
	assert.Empty(t, stored.ExpectedResource)
}
