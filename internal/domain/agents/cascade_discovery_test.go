package agents

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestAgentCascadeNewSubjectAfterDiscoveryFailsClosed(t *testing.T) {
	svc, repo, store, dependents, _ := newLedgerAgent(t)
	agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Agent"}
	repo.agents[agent.ID] = agent
	grant := &storage.UserGrant{ID: id.NewGrantID(), AgentID: agent.ID, Principal: "newly-committed-subject"}
	snapshot := store.Snapshot
	store.Snapshot = func() func() {
		dependents.grants[grant.ID] = grant
		return snapshot()
	}
	require.Error(t, svc.Delete(context.Background(), agent.ID))
	require.Equal(t, agent, repo.agents[agent.ID], "a discovered subject must never be appended after business locks without its gate")
	require.Equal(t, grant, dependents.grants[grant.ID])
	require.Empty(t, store.Events)
}
