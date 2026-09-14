package memory

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestCanonicalIndexesInMemoryRepositories(t *testing.T) {
	ctx := context.Background()

	t.Run("agents replace remove and release canonical IDs", func(t *testing.T) {
		repo := NewAgentRepository()
		canonicalID := "research-agent"
		agent := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &canonicalID, DisplayName: "Research", Description: "Research agent", PermissionSets: testPermissionSets()}
		require.NoError(t, repo.Create(ctx, agent))
		resolved, err := repo.GetByCanonicalID(ctx, canonicalID)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, resolved.ID)

		duplicate := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &canonicalID, DisplayName: "Duplicate", Description: "Duplicate agent", PermissionSets: testPermissionSets()}
		err = repo.Create(ctx, duplicate)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "canonical_id")

		replacement := "research-agent-v2"
		agent.CanonicalID = &replacement
		require.NoError(t, repo.Update(ctx, agent))
		_, err = repo.GetByCanonicalID(ctx, canonicalID)
		require.Error(t, err)
		require.NoError(t, repo.Delete(ctx, agent.ID))
		require.NoError(t, repo.Create(ctx, duplicate))
	})

	t.Run("permission sets isolate canonical IDs and batch lookup", func(t *testing.T) {
		repo := NewPermissionSetRepository()
		canonicalID := "read-repository"
		permissionSet := &storage.PermissionSet{ID: id.NewPermissionSetID(), CanonicalID: &canonicalID, Name: "Read", Description: "Read repository", ServiceScopes: []storage.ServiceScope{{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional}}}
		require.NoError(t, repo.Create(ctx, permissionSet))
		resolved, err := repo.GetByCanonicalID(ctx, canonicalID)
		require.NoError(t, err)
		assert.Equal(t, permissionSet.ID, resolved.ID)
		canonicalIDs, err := repo.GetCanonicalIDs(ctx, []id.PermissionSetID{permissionSet.ID})
		require.NoError(t, err)
		assert.Equal(t, canonicalID, canonicalIDs[permissionSet.ID])
	})

	t.Run("services release canonical IDs after deletion", func(t *testing.T) {
		repo := NewInMemoryThirdpartyOAuth2ProviderRepository()
		canonicalID := "github-service"
		service := &model.ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID(), CanonicalID: &canonicalID, DisplayName: "GitHub", ClientID: "github-client", Secret: model.NewEncryptedSecret([]byte("ciphertext")), IssuerURI: "https://github.com"}
		require.NoError(t, repo.Create(ctx, service))
		resolved, err := repo.GetByCanonicalID(ctx, canonicalID)
		require.NoError(t, err)
		assert.Equal(t, service.ID, resolved.ID)
		require.NoError(t, repo.Delete(ctx, service.ID))
		replacement := service.Copy()
		replacement.ID = id.NewServiceID()
		require.NoError(t, repo.Create(ctx, replacement))
	})
}
