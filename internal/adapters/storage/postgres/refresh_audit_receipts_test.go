//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func TestPGRefreshReceiptStoresOnlyTrustedRequestContext(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	root, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
		ClientIP: "198.51.100.24", UserAgent: "Consent test client", RequestTarget: "/authorize?refresh_token=secret",
	})
	require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, root.AgentID, func(scope context.Context, at time.Time) error {
		return NewRefreshSessionRepo(adapter).RevokeByID(scope, root.ID, at, storage.RefreshReasonProhibitedReuse)
	}))
	var value []byte
	require.NoError(t, adapter.db.GetContext(ctx, &value,
		`SELECT redacted_context FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'prohibited_reuse'`, root.ID))
	var stored map[string]string
	require.NoError(t, json.Unmarshal(value, &stored))
	require.Equal(t, map[string]string{
		"origin": "request", "client_ip": "198.51.100.24", "user_agent": "Consent test client",
	}, stored, "persist only resolved redacted request data, never a token-bearing target")
}

func TestPGRefreshMaintenanceReceiptSurvivesAgentCascade(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	owner, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	neighbor, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.Principal("neighbor@example.test"))
	ctx := security.WithMaintenanceAuditOrigin(security.WithSecurityContext(context.Background(), security.SecurityContext{
		ClientIP: "198.51.100.33", UserAgent: "should not leak",
	}))
	require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
		if err := NewRefreshSessionRepo(adapter).RevokeByAgent(scope, owner.AgentID, at, storage.RefreshReasonAgentDeleted); err != nil {
			return err
		}
		return NewAgentRepository(adapter).Delete(scope, owner.AgentID)
	}))
	for _, root := range []*storage.RefreshSession{owner, neighbor} {
		var value []byte
		require.NoError(t, adapter.db.GetContext(ctx, &value,
			`SELECT redacted_context FROM refresh_revocation_receipts WHERE session_id = $1 AND reason = 'agent_deleted'`, root.ID))
		var stored map[string]string
		require.NoError(t, json.Unmarshal(value, &stored))
		require.Equal(t, map[string]string{"origin": "maintenance"}, stored)
	}
}

func TestPGRefreshListActiveByAgentIsScopedAndNonterminal(t *testing.T) {
	adapter, cleanup := setupMigratedAdapter(t)
	defer cleanup()
	ctx := context.Background()
	owner, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	neighbor, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.Principal("neighbor@example.test"))
	terminal, _ := createPGRefreshRoot(t, adapter, owner.AgentID, id.Principal("owner@example.test"))
	other, _ := createPGRefreshRoot(t, adapter, id.AgentID{}, id.Principal("owner@example.test"))
	require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.AgentID, func(scope context.Context, at time.Time) error {
		return NewRefreshSessionRepo(adapter).RevokeByID(scope, terminal.ID, at, storage.RefreshReasonGrantDeleted)
	}))
	require.NoError(t, NewAuthorizationSessionCoordinator(adapter).Run(ctx, owner.AgentID, func(scope context.Context, _ time.Time) error {
		repo := NewRefreshSessionRepo(adapter)
		own, err := repo.ListActiveByAgent(scope, owner.AgentID, &owner.Principal)
		if err != nil {
			return err
		}
		require.Len(t, own, 1)
		require.Equal(t, owner.AuditIdentity(), own[0])
		all, err := repo.ListActiveByAgent(scope, owner.AgentID, nil)
		if err != nil {
			return err
		}
		require.Len(t, all, 2)
		require.ElementsMatch(t, []storage.RefreshSessionAuditIdentity{owner.AuditIdentity(), neighbor.AuditIdentity()}, all)
		_, err = repo.ListActiveByAgent(scope, other.AgentID, nil)
		require.Error(t, err, "a different agent cannot read another owner's active roots")
		return nil
	}))
	_, err := NewRefreshSessionRepo(adapter).ListActiveByAgent(ctx, owner.AgentID, nil)
	require.Error(t, err, "revocation audit reads require the scoped owner transaction")
}
