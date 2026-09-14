//go:build integration
// +build integration

package postgres

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

func TestCanonicalIDsPostgresRepositories(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	now := time.Now().UTC()

	providers := NewPostgresThirdpartyOAuth2ProviderRepository(adapter)
	servicesCanonicalID := "shared"
	service := &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		CanonicalID: &servicesCanonicalID,
		DisplayName: "Canonical Service",
		ClientID:    "canonical-service-client",
		Secret:      model.NewEncryptedSecret([]byte("ciphertext")),
		IssuerURI:   "https://service.example.com",
		Endpoints:   model.OAuth2Endpoints{TokenEndpoint: "https://service.example.com/token", AuthorizeEndpoint: "https://service.example.com/authorize"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	require.NoError(t, providers.Create(ctx, service))
	resolvedService, err := providers.GetByCanonicalID(ctx, servicesCanonicalID)
	require.NoError(t, err)
	assert.Equal(t, service.ID, resolvedService.ID)

	duplicateService := *service
	duplicateService.ID = id.NewServiceID()
	err = providers.Create(ctx, &duplicateService)
	require.Error(t, err)
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

	permissionSets := NewPermissionSetRepository(adapter)
	permissionCanonicalID := "shared"
	permissionSet := &storage.PermissionSet{
		ID:          id.NewPermissionSetID(),
		CanonicalID: &permissionCanonicalID,
		Name:        "Canonical Permission Set",
		Description: "Canonical permission set",
		ServiceScopes: []storage.ServiceScope{{
			ServiceID: service.ID, RequirementType: storage.RequirementTypeOptional,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, permissionSets.Create(ctx, permissionSet))
	resolvedPermissionSet, err := permissionSets.GetByCanonicalID(ctx, permissionCanonicalID)
	require.NoError(t, err)
	assert.Equal(t, permissionSet.ID, resolvedPermissionSet.ID)
	assert.Equal(t, service.ID, resolvedPermissionSet.ServiceScopes[0].ServiceID)

	duplicatePermissionSet := *permissionSet
	duplicatePermissionSet.ID = id.NewPermissionSetID()
	err = permissionSets.Create(ctx, &duplicatePermissionSet)
	require.Error(t, err)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindConflict, storageErr.Kind)

	missingServicePermissionSet := &storage.PermissionSet{
		ID:          id.NewPermissionSetID(),
		Name:        "Missing Service Permission Set",
		Description: "Must not persist when its service is missing",
		ServiceScopes: []storage.ServiceScope{{
			ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}
	err = permissionSets.Create(ctx, missingServicePermissionSet)
	require.Error(t, err)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindValidation, storageErr.Kind)
	_, err = permissionSets.Get(ctx, missingServicePermissionSet.ID)
	require.Error(t, err)
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindNotFound, storageErr.Kind)

	agents := NewAgentRepository(adapter)
	agentCanonicalID := "shared"
	agent := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &agentCanonicalID, DisplayName: "Canonical Agent", Description: "Canonical agent", CreatedAt: now, UpdatedAt: now}
	attachTestPermissionSet(t, adapter, agent)
	require.NoError(t, agents.Create(ctx, agent))
	resolvedAgent, err := agents.GetByCanonicalID(ctx, agentCanonicalID)
	require.NoError(t, err)
	assert.Equal(t, agent.ID, resolvedAgent.ID)

	duplicate := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &agentCanonicalID, DisplayName: "Duplicate", Description: "Duplicate agent", CreatedAt: now, UpdatedAt: now}
	attachTestPermissionSet(t, adapter, duplicate)
	err = agents.Create(ctx, duplicate)
	require.Error(t, err)
	var agentStorageErr *storage.StorageError
	require.ErrorAs(t, err, &agentStorageErr)
	assert.Equal(t, storage.ErrorKindConflict, agentStorageErr.Kind)

	agent.CanonicalID = nil
	agent.UpdatedAt = time.Now().UTC()
	require.NoError(t, agents.Update(ctx, agent))
	_, err = agents.GetByCanonicalID(ctx, agentCanonicalID)
	require.Error(t, err)
	require.ErrorAs(t, err, &agentStorageErr)
	assert.Equal(t, storage.ErrorKindNotFound, agentStorageErr.Kind)

	reusedCanonicalID := agentCanonicalID
	reused := &storage.Agent{ID: id.NewAgentID(), CanonicalID: &reusedCanonicalID, DisplayName: "Reused Canonical Agent", Description: "Reuses released canonical ID", CreatedAt: now, UpdatedAt: now}
	attachTestPermissionSet(t, adapter, reused)
	require.NoError(t, agents.Create(ctx, reused))
}
