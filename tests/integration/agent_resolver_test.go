package integration

import (
	"context"
	"log/slog"
	"testing"

	memrepo "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopServiceReqValidator satisfies agents.ServiceRequirementValidator for tests
// that don't involve service requirements.
type noopServiceReqValidator struct{}

func (n *noopServiceReqValidator) ValidateServiceRequirements(_ context.Context, _ []storage.ServiceRequirement) error {
	return nil
}

// TestResolveAgentIDByClientID_AmbiguousClientID verifies that the single-agent
// resolver fails loudly when two agents share the same client_id (a state reachable
// when multi_agent_client was previously enabled).
func TestResolveAgentIDByClientID_AmbiguousClientID(t *testing.T) {
	repo := memrepo.NewAgentRepository()
	ctx := context.Background()

	shared := id.ClientID("shared-client-id")
	permissionSets := []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeMandatory,
	}}
	agent1 := &storage.Agent{ID: id.NewAgentID(), ClientID: ptr.To(shared), DisplayName: "A1", Description: "d"}
	agent2 := &storage.Agent{ID: id.NewAgentID(), ClientID: ptr.To(shared), DisplayName: "A2", Description: "d"}
	agent1.PermissionSets = permissionSets
	agent2.PermissionSets = permissionSets
	require.NoError(t, repo.Create(ctx, agent1))
	require.NoError(t, repo.Create(ctx, agent2))

	svc := agents.NewService(repo, &noopServiceReqValidator{}, slog.Default(), false)

	_, err := svc.ResolveUniqueByClientID(ctx, shared)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

// TestResolveAgentIDByClientID_UnambiguousClientID verifies the happy path:
// a unique client_id resolves to the registered agent's UUID.
func TestResolveAgentIDByClientID_UnambiguousClientID(t *testing.T) {
	repo := memrepo.NewAgentRepository()
	ctx := context.Background()
	permissionSets := []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeMandatory,
	}}

	agentID := id.NewAgentID()
	agent := &storage.Agent{ID: agentID, ClientID: ptr.To(id.ClientID("unique-client")), DisplayName: "A", Description: "d"}
	agent.PermissionSets = permissionSets
	require.NoError(t, repo.Create(ctx, agent))

	svc := agents.NewService(repo, &noopServiceReqValidator{}, slog.Default(), false)

	resolved, err := svc.ResolveUniqueByClientID(ctx, "unique-client")
	require.NoError(t, err)
	assert.Equal(t, agentID.String(), resolved.ID.String())
}
