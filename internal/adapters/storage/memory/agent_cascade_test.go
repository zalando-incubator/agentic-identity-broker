package memory_test

import (
	"context"
	"log/slog"
	"testing"

	eventschemas "github.com/agentic-identity-broker/agentic-identity-broker/api/events"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
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
			deleted, err := f.adapter.Agents().Delete(ctx, f.agent.ID)
			require.NoError(t, err)
			require.True(t, deleted)
			_, err = f.adapter.UserGrants().Get(ctx, grant.ID)
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

func TestMemoryAgentCascadeInvalidatesApprovalSyncOnCommitOnly(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[commit], func(t *testing.T) {
			f := newTransactionFixture(t)
			pending := f.approval(t)
			registry, err := ledger.NewRegistry(eventschemas.Schemas)
			require.NoError(t, err)
			recorder := ledger.NewService(registry, f.adapter.BusinessEvents(), nil, f.adapter, false)
			broadcaster := approval.NewApprovalSyncBroadcaster(0)
			updates := broadcaster.Subscribe()
			defer broadcaster.Unsubscribe(updates)
			service := agents.NewService(f.adapter.Agents(), nil, slog.New(slog.DiscardHandler), true, nil, recorder,
				f.adapter.Agents().(ports.AgentDependentRepository), f.adapter.BrokerCredentials(), f.adapter.ApprovalSyncState(), broadcaster)
			ctx := f.begin(t)
			require.NoError(t, service.Delete(ctx, f.agent.ID))
			version, err := f.adapter.ApprovalSyncState().GetVersion(ctx)
			require.NoError(t, err)
			require.EqualValues(t, 1, version)
			require.Empty(t, updates, "uncommitted cascade must not wake subscribers")
			if commit {
				require.NoError(t, f.adapter.Commit(ctx))
				require.Len(t, updates, 1)
			} else {
				require.NoError(t, f.adapter.Rollback(ctx))
				require.Empty(t, updates)
			}
			version, err = f.adapter.ApprovalSyncState().GetVersion(context.Background())
			require.NoError(t, err)
			_, err = f.adapter.ToolApprovals().Get(context.Background(), pending.ID)
			if commit {
				require.EqualValues(t, 1, version)
				require.True(t, ports.IsNotFoundErr(err))
			} else {
				require.Zero(t, version)
				require.NoError(t, err)
			}
		})
	}
}
