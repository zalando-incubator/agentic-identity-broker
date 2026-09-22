//go:build integration
// +build integration

package postgres

import (
	"context"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		newGrantedPermissionSetEntry(t, adapter, provider.ID),
	)
	require.NoError(t, NewUserGrantRepository(adapter).Create(ctx, grant))

	err := repo.Delete(ctx, provider.ID)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)
	assert.Contains(t, storageErr.Error(), "user grant")
	_, err = repo.Get(ctx, provider.ID)
	require.NoError(t, err)
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
