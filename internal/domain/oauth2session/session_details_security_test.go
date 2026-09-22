package oauth2session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestGetSessionWithAgentsSecurity_PrincipalIsolation(t *testing.T) {
	ctx := context.Background()
	service, _, sessionRepo, grantRepo, agentRepo, _ := setupServiceWithConfig(t, nil)
	principalA := id.Principal("principal-a@example.com")
	principalB := id.Principal("principal-b@example.com")
	serviceID := id.NewServiceID()
	permissionSetAID := id.NewPermissionSetID()
	permissionSetBID := id.NewPermissionSetID()
	now := time.Now()

	for _, session := range []*storage.UserSession{
		{ID: id.NewSessionID(), Principal: principalA, ServiceID: serviceID, EncryptedAccessToken: []byte("token-a"), TokenType: "Bearer", InitiatedAt: now, CreatedAt: now},
		{ID: id.NewSessionID(), Principal: principalB, ServiceID: serviceID, EncryptedAccessToken: []byte("token-b"), TokenType: "Bearer", InitiatedAt: now, CreatedAt: now},
	} {
		require.NoError(t, sessionRepo.Create(ctx, session))
	}

	agentA := &storage.Agent{
		ID:          id.NewAgentID(),
		DisplayName: "Agent A",
		Description: "Principal A agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetAID,
			RequirementType: storage.RequirementTypeMandatory,
		}},
	}
	agentB := &storage.Agent{
		ID:          id.NewAgentID(),
		DisplayName: "Agent B",
		Description: "Principal B agent",
		PermissionSets: []storage.AgentPermissionSetEntry{{
			PermissionSetID: permissionSetBID,
			RequirementType: storage.RequirementTypeMandatory,
		}},
	}
	require.NoError(t, agentRepo.Create(ctx, agentA))
	require.NoError(t, agentRepo.Create(ctx, agentB))

	for _, grant := range []*storage.UserGrant{
		{Principal: principalA, AgentID: agentA.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionSetAID, IncludedServiceIDs: []id.ServiceID{serviceID}}}},
		{Principal: principalB, AgentID: agentB.ID, GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionSetBID, IncludedServiceIDs: []id.ServiceID{serviceID}}}},
	} {
		require.NoError(t, grantRepo.Create(ctx, grant))
	}

	details, err := service.GetSessionWithAgents(ctx, principalA, serviceID)
	require.NoError(t, err)
	require.Len(t, details.DependentAgents, 1)
	assert.Equal(t, agentA.ID, details.DependentAgents[0].ID)
	assert.Equal(t, agentA.DisplayName, details.DependentAgents[0].DisplayName)
	assert.NotEqual(t, agentB.ID, details.DependentAgents[0].ID)
	assert.NotEqual(t, agentB.DisplayName, details.DependentAgents[0].DisplayName)
}
