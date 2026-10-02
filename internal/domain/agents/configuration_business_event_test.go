package agents

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestAgentLedgerCollectionChangesAndEquivalentOrdering(t *testing.T) {
	svc, repo, store, _, _ := newLedgerAgent(t)
	agent := &storage.Agent{DisplayName: "Agent", Description: "Description", PermissionSets: testPermissionSets(), AllowedScopes: []string{"read", "read"}}
	require.NoError(t, svc.Create(context.Background(), agent))
	changed := agent.Copy()
	changed.AllowedScopes = []string{"read", "write"}
	require.NoError(t, svc.Update(context.Background(), agent.ID, changed, false))
	require.Equal(t, []string{"read", "write"}, repo.agents[agent.ID].AllowedScopes)
	require.Len(t, store.Events, 2)
	reordered := changed.Copy()
	reordered.AllowedScopes = []string{"write", "read"}
	require.NoError(t, svc.Update(context.Background(), agent.ID, reordered, false))
	require.Len(t, store.Events, 2, "scope ordering does not change effective authorization")
}
