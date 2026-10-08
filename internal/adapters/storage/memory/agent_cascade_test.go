package memory_test

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestMemoryAgentCascadeDeletionAndRollback(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			f := newTransactionFixture(t)
			grant, approval := f.grant(t), f.approval(t)
			credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: f.agent.ID, SecretHash: "credential-hash", CreatedAt: f.now}
			require.NoError(t, f.adapter.BrokerCredentials().Create(context.Background(), credential))
			ctx := f.begin(t)
			require.NoError(t, f.adapter.Agents().Delete(ctx, f.agent.ID))
			_, err := f.adapter.UserGrants().Get(ctx, grant.ID)
			require.True(t, ports.IsNotFoundErr(err), "agent deletion must remove its grant")
			_, err = f.adapter.ToolApprovals().Get(ctx, approval.ID)
			require.True(t, ports.IsNotFoundErr(err), "agent deletion must remove its approval")
			_, err = f.adapter.BrokerCredentials().GetByAgentID(ctx, f.agent.ID)
			require.True(t, ports.IsNotFoundErr(err), "agent deletion must remove its credential")
			if commit {
				require.NoError(t, f.adapter.Commit(ctx))
			} else {
				require.NoError(t, f.adapter.Rollback(ctx))
				stored, err := f.adapter.Agents().Get(context.Background(), f.agent.ID)
				require.NoError(t, err)
				require.Equal(t, f.agent.ID, stored.ID)
				restoredGrant, err := f.adapter.UserGrants().Get(context.Background(), grant.ID)
				require.NoError(t, err)
				require.Equal(t, grant.ID, restoredGrant.ID)
				restoredApproval, err := f.adapter.ToolApprovals().Get(context.Background(), approval.ID)
				require.NoError(t, err)
				require.Equal(t, approval.ID, restoredApproval.ID)
				restoredCredential, err := f.adapter.BrokerCredentials().GetByAgentID(context.Background(), f.agent.ID)
				require.NoError(t, err)
				require.Equal(t, credential.ID, restoredCredential.ID)
			}
		})
	}
}
