package integration

import (
	"context"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainStorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentRepositoryServiceRequirements_Memory tests service requirements with the in-memory adapter.
func TestAgentRepositoryServiceRequirements_Memory(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  0,
			Write: 0,
		},
	}
	adapter, err := storageadapter.NewAdapter(config)
	require.NoError(t, err)

	permissionSets := []domainStorage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: domainStorage.RequirementTypeMandatory,
	}}

	t.Run("create agent with service requirements", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		agentID := id.MustParseAgentID("a1234567-0001-0001-0001-000000000001")
		serviceID := id.MustParseServiceID("c1234567-0001-0001-0001-000000000001")

		agent := &domainStorage.Agent{
			ID:             agentID,
			ClientID:       ptr.To(id.ClientID("client-sr-1")),
			DisplayName:    "Service Requirements Agent",
			Description:    "Agent with service requirements",
			PermissionSets: permissionSets,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ServiceRequirements: []domainStorage.ServiceRequirement{
				{
					ServiceID:       serviceID,
					RequirementType: domainStorage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo", "user:email"},
				},
			},
		}

		err := adapter.Agents().Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := adapter.Agents().Get(ctx, agent.ID)
		require.NoError(t, err)
		require.NotNil(t, retrieved)
		assert.Equal(t, agent.ClientID, retrieved.ClientID)
		assert.Len(t, retrieved.ServiceRequirements, 1)
		assert.Equal(t, serviceID, retrieved.ServiceRequirements[0].ServiceID)
		assert.Equal(t, domainStorage.RequirementTypeMandatory, retrieved.ServiceRequirements[0].RequirementType)
		assert.Equal(t, []string{"repo", "user:email"}, retrieved.ServiceRequirements[0].RequiredScopes)
	})

	t.Run("update agent service requirements", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		agentID := id.MustParseAgentID("a1234567-0002-0002-0002-000000000002")
		serviceID1 := id.MustParseServiceID("c1234567-0001-0001-0001-000000000001")
		serviceID2 := id.MustParseServiceID("c1234567-0002-0002-0002-000000000002")

		agent := &domainStorage.Agent{
			ID:             agentID,
			ClientID:       ptr.To(id.ClientID("client-sr-2")),
			DisplayName:    "Update Test Agent",
			Description:    "Agent for update testing",
			PermissionSets: permissionSets,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ServiceRequirements: []domainStorage.ServiceRequirement{
				{
					ServiceID:       serviceID1,
					RequirementType: domainStorage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo"},
				},
			},
		}

		err := adapter.Agents().Create(ctx, agent)
		require.NoError(t, err)

		agent.ServiceRequirements = []domainStorage.ServiceRequirement{
			{
				ServiceID:       serviceID2,
				RequirementType: domainStorage.RequirementTypeOptional,
				RequiredScopes:  []string{"read:user"},
			},
		}
		agent.UpdatedAt = time.Now().UTC()

		err = adapter.Agents().Update(ctx, agent)
		require.NoError(t, err)

		retrieved, err := adapter.Agents().Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved.ServiceRequirements, 1)
		assert.Equal(t, serviceID2, retrieved.ServiceRequirements[0].ServiceID)
		assert.Equal(t, domainStorage.RequirementTypeOptional, retrieved.ServiceRequirements[0].RequirementType)
	})

	t.Run("agent with null service requirements (backward compatibility)", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		agent := &domainStorage.Agent{
			ID:             id.MustParseAgentID("a1234567-0003-0003-0003-000000000003"),
			ClientID:       ptr.To(id.ClientID("client-sr-3")),
			DisplayName:    "Legacy Agent",
			Description:    "Agent without service requirements",
			PermissionSets: permissionSets,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}

		err := adapter.Agents().Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := adapter.Agents().Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Nil(t, retrieved.ServiceRequirements)
	})

	t.Run("agent with empty service requirements array", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		agent := &domainStorage.Agent{
			ID:                  id.MustParseAgentID("a1234567-0004-0004-0004-000000000004"),
			ClientID:            ptr.To(id.ClientID("client-sr-4")),
			DisplayName:         "Empty Requirements Agent",
			Description:         "Agent with empty requirements array",
			PermissionSets:      permissionSets,
			CreatedAt:           time.Now().UTC(),
			UpdatedAt:           time.Now().UTC(),
			ServiceRequirements: []domainStorage.ServiceRequirement{},
		}

		err := adapter.Agents().Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := adapter.Agents().Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.NotNil(t, retrieved.ServiceRequirements)
		assert.Len(t, retrieved.ServiceRequirements, 0)
	})

	t.Run("agent with multiple service requirements", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		githubSvcID := id.MustParseServiceID("c1234567-1111-1111-1111-111111111111")
		gitlabSvcID := id.MustParseServiceID("c1234567-2222-2222-2222-222222222222")
		slackSvcID := id.MustParseServiceID("c1234567-3333-3333-3333-333333333333")

		agent := &domainStorage.Agent{
			ID:             id.MustParseAgentID("a1234567-0005-0005-0005-000000000005"),
			ClientID:       ptr.To(id.ClientID("client-sr-5")),
			DisplayName:    "Multi Service Agent",
			Description:    "Agent with multiple service requirements",
			PermissionSets: permissionSets,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			ServiceRequirements: []domainStorage.ServiceRequirement{
				{
					ServiceID:       githubSvcID,
					RequirementType: domainStorage.RequirementTypeMandatory,
					RequiredScopes:  []string{"repo", "user:email"},
				},
				{
					ServiceID:       gitlabSvcID,
					RequirementType: domainStorage.RequirementTypeOptional,
					RequiredScopes:  []string{"api", "read_user"},
				},
				{
					ServiceID:       slackSvcID,
					RequirementType: domainStorage.RequirementTypeMandatory,
					RequiredScopes:  []string{"users:read"},
				},
			},
		}

		err := adapter.Agents().Create(ctx, agent)
		require.NoError(t, err)

		retrieved, err := adapter.Agents().Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Len(t, retrieved.ServiceRequirements, 3)

		for i, req := range retrieved.ServiceRequirements {
			assert.Equal(t, agent.ServiceRequirements[i].ServiceID, req.ServiceID)
			assert.Equal(t, agent.ServiceRequirements[i].RequirementType, req.RequirementType)
			assert.Equal(t, agent.ServiceRequirements[i].RequiredScopes, req.RequiredScopes)
		}
	})
}
