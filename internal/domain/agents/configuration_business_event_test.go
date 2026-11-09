package agents

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/require"
)

func TestAgentLedgerCollectionChangesAndEquivalentOrdering(t *testing.T) {
	svc, repo, store, _, _ := newLedgerAgent(t)
	agent := &storage.Agent{DisplayName: "Agent", Description: "Description", PermissionSets: testPermissionSets(), AllowedScopes: []string{"read", "read"}}
	require.NoError(t, svc.Create(context.Background(), agent))
	changed := agent.Copy()
	changed.AllowedScopes = []string{"read", "write"}
	require.NoError(t, svc.Update(context.Background(), agent.ID, changed, UpdateInput{}))
	require.Equal(t, []string{"read", "write"}, repo.agents[agent.ID].AllowedScopes)
	require.Len(t, store.Events, 2)
	reordered := changed.Copy()
	reordered.AllowedScopes = []string{"write", "read"}
	require.NoError(t, svc.Update(context.Background(), agent.ID, reordered, UpdateInput{}))
	require.Len(t, store.Events, 2, "scope ordering does not change effective authorization")
}

func TestAgentLedgerSingleFieldConfigurationChanges(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*storage.Agent)
	}{
		{"canonical ID", func(a *storage.Agent) { a.CanonicalID = ptr.To("changed-agent") }},
		{"clear canonical ID", func(a *storage.Agent) { a.CanonicalID = nil }},
		{"client ID", func(a *storage.Agent) { a.ClientID = ptr.To(id.ClientID("changed-client")) }},
		{"external ID", func(a *storage.Agent) { a.ExternalID = ptr.To(id.ExternalID("changed-external")) }},
		{"display name", func(a *storage.Agent) { a.DisplayName = "Changed Agent" }},
		{"description", func(a *storage.Agent) { a.Description = "Changed description" }},
		{"governance URL", func(a *storage.Agent) { a.GovernanceURL = ptr.To("https://example.com/governance") }},
		{"documentation URL", func(a *storage.Agent) { a.UserDocumentationURL = ptr.To("https://example.com/docs") }},
		{"interface URL", func(a *storage.Agent) { a.AgentInterfaceURL = ptr.To("https://example.com/interface") }},
		{"redirect URIs", func(a *storage.Agent) { a.RedirectURIs = []string{"https://example.com/callback"} }},
		{"allowed scopes", func(a *storage.Agent) { a.AllowedScopes = []string{"write"} }},
		{"client URIs", func(a *storage.Agent) { a.ClientURIs = []string{"https://example.com/client.json"} }},
		{"permission set ID", func(a *storage.Agent) { a.PermissionSets[0].PermissionSetID = id.NewPermissionSetID() }},
		{"permission set requirement", func(a *storage.Agent) { a.PermissionSets[0].RequirementType = storage.RequirementTypeMandatory }},
		{"service ID", func(a *storage.Agent) { a.ServiceRequirements[0].ServiceID = id.NewServiceID() }},
		{"service requirement", func(a *storage.Agent) { a.ServiceRequirements[0].RequirementType = storage.RequirementTypeMandatory }},
		{"required scopes", func(a *storage.Agent) { a.ServiceRequirements[0].RequiredScopes = []string{"read"} }},
		{"require all scopes", func(a *storage.Agent) { a.ServiceRequirements[0].RequireAllScopes = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, store, _, _ := newLedgerAgent(t)
			ctx := context.Background()
			agent := &storage.Agent{CanonicalID: ptr.To("original-agent"), DisplayName: "Agent", Description: "Description", PermissionSets: testPermissionSets(), AllowedScopes: []string{"read"},
				ServiceRequirements: []storage.ServiceRequirement{{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional}}}
			svc.permissionSets = coveredPermissionSets(agent.ServiceRequirements)
			require.NoError(t, svc.Create(ctx, agent))
			changed := agent.Copy()
			tc.change(changed)
			svc.permissionSets = coveredPermissionSets(changed.ServiceRequirements)
			require.NoError(t, svc.Update(ctx, agent.ID, changed, UpdateInput{CanonicalIDPresent: tc.name == "clear canonical ID"}))
			stored, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			require.Equal(t, changed.Copy(), stored)
			require.Len(t, store.Events, 2)
			require.Equal(t, model.BusinessEventTypePrefix+"agent-updated", store.Events[1].Type)
			require.Equal(t, agent.ID, store.Events[1].AgentID)
			require.NoError(t, svc.Update(ctx, agent.ID, stored.Copy(), UpdateInput{}))
			require.Len(t, store.Events, 2, "repeating effective configuration must remain silent")
		})
	}
}

func TestAgentLedgerEquivalentConfigurationOrderingsRemainSilent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		reorder func(*storage.Agent)
	}{
		{"allowed scopes", func(a *storage.Agent) {
			a.AllowedScopes[0], a.AllowedScopes[1] = a.AllowedScopes[1], a.AllowedScopes[0]
		}},
		{"client URIs", func(a *storage.Agent) { a.ClientURIs[0], a.ClientURIs[1] = a.ClientURIs[1], a.ClientURIs[0] }},
		{"permission sets", func(a *storage.Agent) {
			a.PermissionSets[0], a.PermissionSets[1] = a.PermissionSets[1], a.PermissionSets[0]
		}},
		{"service requirements", func(a *storage.Agent) {
			a.ServiceRequirements[0], a.ServiceRequirements[1] = a.ServiceRequirements[1], a.ServiceRequirements[0]
		}},
		{"required scopes", func(a *storage.Agent) {
			a.ServiceRequirements[0].RequiredScopes[0], a.ServiceRequirements[0].RequiredScopes[1] = a.ServiceRequirements[0].RequiredScopes[1], a.ServiceRequirements[0].RequiredScopes[0]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, store, _, _ := newLedgerAgent(t)
			ctx := context.Background()
			agent := &storage.Agent{DisplayName: "Agent", Description: "Description", PermissionSets: append(testPermissionSets(), testPermissionSets()...), AllowedScopes: []string{"read", "write"},
				RedirectURIs: []string{"https://example.com/first", "https://example.com/second"}, ClientURIs: []string{"https://example.com/first.json", "https://example.com/second.json"},
				ServiceRequirements: []storage.ServiceRequirement{{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"read", "write"}}, {ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory}}}
			svc.permissionSets = coveredPermissionSets(agent.ServiceRequirements)
			require.NoError(t, svc.Create(ctx, agent))
			reordered := agent.Copy()
			tc.reorder(reordered)
			require.NoError(t, svc.Update(ctx, agent.ID, reordered, UpdateInput{}))
			stored, err := repo.Get(ctx, agent.ID)
			require.NoError(t, err)
			require.Equal(t, agent, stored)
			require.Len(t, store.Events, 1)
			require.Equal(t, model.BusinessEventTypePrefix+"agent-registered", store.Events[0].Type)
		})
	}
}

func TestAgentNoOpUpdateReturnsPersistedState(t *testing.T) {
	svc, repo, store, _, _ := newLedgerAgent(t)
	ctx := context.Background()
	agent := &storage.Agent{DisplayName: "Agent", Description: "Description", PermissionSets: testPermissionSets(), UpdatedAt: time.Now().UTC().Add(-time.Hour)}
	require.NoError(t, svc.Create(ctx, agent))
	request := agent.Copy()
	request.UpdatedAt = time.Now().UTC()
	require.NoError(t, svc.Update(ctx, agent.ID, request, UpdateInput{}))
	stored, err := repo.Get(ctx, agent.ID)
	require.NoError(t, err)
	require.Equal(t, stored, request)
	require.Len(t, store.Events, 1)
}
