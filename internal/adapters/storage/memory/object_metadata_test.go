package memory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func memoryMetadata(t *testing.T, tier string) model.ObjectMetadata {
	t.Helper()
	value, err := model.NewObjectMetadata(map[string]string{"tier": tier, "Exposure": "public"}, map[string]string{"note": "東京"})
	require.NoError(t, err)
	return value
}

func TestMemoryAgentObjectMetadata(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentRepository()
	metadata := memoryMetadata(t, "gold")
	clientID := id.ClientID("metadata-client")
	external := id.ExternalID("registry/metadata")
	first := &storage.Agent{ID: id.NewAgentID(), ClientID: &clientID, ExternalID: &external, DisplayName: "A", Description: "A", PermissionSets: testPermissionSets(), ObjectMetadata: metadata}
	clientURI := "https://agent.example.test/metadata.json"
	second := &storage.Agent{ID: id.NewAgentID(), ClientURIs: []string{clientURI}, DisplayName: "B", Description: "B", PermissionSets: testPermissionSets(), ObjectMetadata: metadata}
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	reads := []func() (*storage.Agent, error){
		func() (*storage.Agent, error) { return repo.Get(ctx, first.ID) },
		func() (*storage.Agent, error) { return repo.GetByExternalID(ctx, external) },
		func() (*storage.Agent, error) { return repo.GetByClientID(ctx, clientID) },
		func() (*storage.Agent, error) { return repo.GetByClientURI(ctx, clientURI) },
		func() (*storage.Agent, error) {
			agents, err := repo.GetByIDs(ctx, []id.AgentID{first.ID})
			require.Len(t, agents, 1)
			return agents[0], err
		},
	}
	for _, read := range reads {
		agent, err := read()
		require.NoError(t, err)
		assert.True(t, metadata.Equal(agent.ObjectMetadata))
	}
	listed, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	updated := first.Copy()
	updated.ObjectMetadata = memoryMetadata(t, "silver")
	require.NoError(t, repo.Update(ctx, updated))
	storedFirst, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	storedSecond, err := repo.Get(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, "silver", storedFirst.ObjectMetadata.Labels()["tier"])
	assert.Equal(t, "gold", storedSecond.ObjectMetadata.Labels()["tier"], "agents with equal maps stay independent")
}

func TestMemoryPermissionSetObjectMetadata(t *testing.T) {
	ctx := context.Background()
	repo := NewPermissionSetRepository()
	metadata := memoryMetadata(t, "gold")
	serviceID := id.NewServiceID()
	newSet := func(name string) *storage.PermissionSet {
		return &storage.PermissionSet{
			ID: id.NewPermissionSetID(), Name: name, Description: name, ObjectMetadata: metadata,
			ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory}},
		}
	}
	first, second := newSet("first"), newSet("second")
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))
	stored, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	assert.True(t, metadata.Equal(stored.ObjectMetadata))
	batch, err := repo.GetByIDs(ctx, []id.PermissionSetID{first.ID})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	assert.True(t, metadata.Equal(batch[0].ObjectMetadata))
	for _, service := range []id.ServiceID{{}, serviceID} {
		listed, err := repo.List(ctx, service)
		require.NoError(t, err)
		require.Len(t, listed, 2)
		for _, ps := range listed {
			assert.True(t, metadata.Equal(ps.ObjectMetadata))
		}
	}

	updated := first.Copy()
	updated.ObjectMetadata = model.ObjectMetadata{}
	updated.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.Update(ctx, updated))
	storedFirst, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	storedSecond, err := repo.Get(ctx, second.ID)
	require.NoError(t, err)
	assert.True(t, storedFirst.ObjectMetadata.IsEmpty())
	assert.True(t, metadata.Equal(storedSecond.ObjectMetadata), "permission sets with equal maps stay independent")
}

func TestMemoryServiceObjectMetadata(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
	metadata := memoryMetadata(t, "gold")
	newService := func(resource string) *model.ThirdpartyOAuth2ProviderEntity {
		return &model.ThirdpartyOAuth2ProviderEntity{
			ID: id.NewServiceID(), DisplayName: "Service", ClientID: "client-" + id.ClientID(resource),
			Secret: model.NewEncryptedSecret([]byte("opaque")), IssuerURI: "https://issuer.example.test",
			Endpoints:          model.OAuth2Endpoints{TokenEndpoint: "https://issuer.example.test/token", AuthorizeEndpoint: "https://issuer.example.test/authorize"},
			Scopes:             []model.OAuthScope{{ScopeValue: "read", Description: "Read"}},
			ProtectedResources: []string{"https://api.example.test/" + resource},
			ObjectMetadata:     metadata, ObjectMetadataProvided: true,
		}
	}
	first, second := newService("first"), newService("second")
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Create(ctx, second))

	check := func(entity *model.ThirdpartyOAuth2ProviderEntity, want model.ObjectMetadata) {
		t.Helper()
		assert.True(t, want.Equal(entity.ObjectMetadata))
		assert.False(t, entity.ObjectMetadataProvided, "the stored record never keeps the transient flag")
	}
	stored, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	check(stored, metadata)
	found, err := repo.FindByProtectedResource(ctx, first.ProtectedResources[0])
	require.NoError(t, err)
	check(found, metadata)
	batch, err := repo.GetByIDs(ctx, []id.ServiceID{first.ID})
	require.NoError(t, err)
	require.Len(t, batch, 1)
	check(batch[0], metadata)
	listed, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	before := stored.Version
	stored.ObjectMetadata = memoryMetadata(t, "silver")
	require.NoError(t, repo.Update(ctx, stored, &before))
	updated, err := repo.Get(ctx, first.ID)
	require.NoError(t, err)
	check(updated, memoryMetadata(t, "silver"))
	assert.Equal(t, before+1, updated.Version)
	other, err := repo.Get(ctx, second.ID)
	require.NoError(t, err)
	check(other, metadata)
}
