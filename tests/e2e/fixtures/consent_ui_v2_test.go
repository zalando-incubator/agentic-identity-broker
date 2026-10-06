package fixtures

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestConsentV2FixturesReconstructIsolatedVisualState(t *testing.T) {
	principal := id.Principal(DefaultPrincipal().String())
	otherPrincipal := id.Principal(AnotherPrincipal().String())
	first := ConsentV2Service("Project files", "https://provider.example")
	second := ConsentV2Service("Work calendar", "https://provider.example")
	missing := ConsentV2Service("Unconnected provider", "https://provider.example")
	// Ephemeral upstream origins must not change the visible provider identity.
	reconstructedFirst := ConsentV2Service(first.DisplayName, "https://another-provider.example")
	require.Equal(t, first.ID, reconstructedFirst.ID)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, "https://another-provider.example/oauth/authorize", reconstructedFirst.Endpoints.AuthorizeEndpoint)

	sets := ConsentV2PermissionSets(first.ID, second.ID)
	reconstructedSets := ConsentV2PermissionSets(reconstructedFirst.ID, second.ID)
	require.Equal(t, sets, reconstructedSets)
	seenSets := make(map[id.PermissionSetID]bool)
	for _, set := range append(sets, ConsentV2PermissionSets(missing.ID, second.ID)...) {
		require.NoError(t, set.Validate())
		require.False(t, seenSets[set.ID], "normal and disconnected permission sets must coexist")
		seenSets[set.ID] = true
	}

	agent := ConsentV2Agent("Returning assistant", "https://callback.example", sets)
	reconstructedAgent := ConsentV2Agent(agent.DisplayName, "https://callback.example", reconstructedSets)
	require.Equal(t, agent, reconstructedAgent)
	seenAgents := map[id.AgentID]bool{agent.ID: true}
	for _, delegated := range ConsentV2GrantedAgents("https://callback.example", sets) {
		require.NoError(t, delegated.Validate())
		require.False(t, seenAgents[delegated.ID], "all delegated cards need distinct identities")
		seenAgents[delegated.ID] = true
	}

	grant := ConsentV2DeltaGrant(principal, agent, sets)
	reconstructedGrant := ConsentV2DeltaGrant(principal, reconstructedAgent, reconstructedSets)
	require.Equal(t, grant, reconstructedGrant)
	privateGrant := ConsentV2DeltaGrant(otherPrincipal, agent, sets)
	require.NotEqual(t, grant.ID, privateGrant.ID)
	require.Equal(t, otherPrincipal, privateGrant.Principal)

	approval := ConsentV2Approval(principal, agent.ID, "read_roadmap", "high")
	reconstructedApproval := ConsentV2Approval(principal, reconstructedAgent.ID, "read_roadmap", "high")
	require.Equal(t, approval, reconstructedApproval)
	require.NotEqual(t, approval.ID, ConsentV2Approval(principal, agent.ID, "unrated_roadmap", "").ID)
	require.NotEqual(t, approval.ID, ConsentV2Approval(otherPrincipal, agent.ID, "read_roadmap", "high").ID)

	session := ConsentV2Session(principal, first.ID, "usable")
	reconstructedSession := ConsentV2Session(principal, reconstructedFirst.ID, "usable")
	require.Equal(t, session.ID, reconstructedSession.ID)
	require.Equal(t, session.InitiatedAt, reconstructedSession.InitiatedAt)
	require.NotEqual(t, session.ID, ConsentV2Session(otherPrincipal, first.ID, "usable").ID)

	// Stable identities must not turn fresh scenarios into shared mutable state.
	agent.PermissionSets[0].RequirementType = storage.RequirementTypeMandatory
	sets[0].ServiceScopes[0].Scopes[0] = "changed"
	grant.GrantedPermissionSets[0].IncludedServiceIDs[0] = missing.ID
	approval.Arguments["path"] = "/changed"
	require.NotEqual(t, agent.PermissionSets, reconstructedAgent.PermissionSets)
	require.Equal(t, "read", reconstructedSets[0].ServiceScopes[0].Scopes[0])
	require.Equal(t, first.ID, reconstructedGrant.GrantedPermissionSets[0].IncludedServiceIDs[0])
	require.Equal(t, "/projects/roadmap", reconstructedApproval.Arguments["path"])
}
